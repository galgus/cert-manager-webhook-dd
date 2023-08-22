package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/cert-manager/cert-manager/pkg/issuer/acme/dns/util"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our dondominio DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&ddDNSProviderSolver{},
	)
}

// ddDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/jetstack/cert-manager/pkg/acme/webhook.Solver`
// interface.
type ddDNSProviderSolver struct {
	client *kubernetes.Clientset
}

// ddDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type ddDNSProviderConfig struct {
	Endpoint             string                   `json:"endpoint"`
	ApplicationKey       string                   `json:"applicationKey"`
	ApplicationSecretRef corev1.SecretKeySelector `json:"applicationSecretRef"`
}

type ddServiceInfo struct {
	Success      bool                  `json:"success"`
	ErrorCode    int64                 `json:"errorCode"`
	ErrorCodeMsg string                `json:"errorCodeMsg"`
	Action       string                `json:"action"`
	Version      string                `json:"version"`
	ResponseData ddServiceInfoResponse `json:"responseData"`
}

type ddServiceList struct {
	Success      bool                  `json:"success"`
	ErrorCode    int64                 `json:"errorCode"`
	ErrorCodeMsg string                `json:"errorCodeMsg"`
	Action       string                `json:"action"`
	Version      string                `json:"version"`
	Messages     []string              `json:"messages,omitempty"`
	ResponseData ddServiceListResponse `json:"responseData"`
}

type ddServiceInfoResponse struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Productkey  string `json:"productkey"`
	Status      string `json:"status"`
	TsExpir     string `json:"tsExpir"`
	TsCreate    string `json:"tsCreate"`
	Renewable   bool   `json:"renewable"`
	RenewalMode string `json:"renewalMode"`
}

type ddServiceListResponse struct {
	QueryInfo QueryInfo `json:"queryInfo,omitempty"`
	Dns       []Dns     `json:"dns"`
}

type QueryInfo struct {
	Page       uint64 `json:"page"`
	PageLength uint64 `json:"pageLength"`
	Results    uint64 `json:"results"`
	Total      uint64 `json:"total"`
}

type Dns struct {
	EntityID string `json:"entityID"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Ttl      string `json:"ttl"`
	Priority string `json:"priority"`
	Value    string `json:"value"`
}

type ddServiceStatusParams struct {
	ServiceName string `schema:"serviceName"`
	InfoType    string `schema:"infoType"`
}

type ddCreateServiceParams struct {
	FieldType   string `schema:"type"`
	ServiceName string `schema:"serviceName"`
	Name        string `schema:"name"`
	Value       string `schema:"value"`
}

type ddServiceListParams struct {
	ServiceName string `schema:"serviceName"`
	FilterValue string `schema:"filterValue"`
}

type ddDeleteServiceParams struct {
	ServiceName string `schema:"serviceName"`
	EntityId    string `schema:"entityID"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (s *ddDNSProviderSolver) Name() string {
	return "don-dominio"
}

func (s *ddDNSProviderSolver) validate(cfg *ddDNSProviderConfig, allowAmbientCredentials bool) error {
	if allowAmbientCredentials {
		// When allowAmbientCredentials is true, DD client can load missing config
		// values from the environment variables and the dondominio.conf files.
		return nil
	}
	if cfg.Endpoint == "" {
		return errors.New("no endpoint provided in DonDominio config")
	}
	if cfg.ApplicationKey == "" {
		return errors.New("no application key provided in DonDominio config")
	}
	if cfg.ApplicationSecretRef.Name == "" {
		return errors.New("no application secret provided in DonDominio config")
	}
	return nil
}

func (s *ddDNSProviderSolver) ddClient(ch *v1alpha1.ChallengeRequest) (*Client, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return nil, err
	}

	err = s.validate(&cfg, ch.AllowAmbientCredentials)
	if err != nil {
		return nil, err
	}

	applicationSecret, err := s.secret(cfg.ApplicationSecretRef, ch.ResourceNamespace)
	if err != nil {
		return nil, err
	}

	return NewClient(cfg.Endpoint, cfg.ApplicationKey, applicationSecret)
}

func (s *ddDNSProviderSolver) secret(ref corev1.SecretKeySelector, namespace string) (string, error) {
	if ref.Name == "" {
		return "", nil
	}

	secret, err := s.client.CoreV1().Secrets(namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	bytes, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key not found %q in secret '%s/%s'", ref.Key, namespace, ref.Name)
	}
	return strings.TrimSuffix(string(bytes), "\n"), nil
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (s *ddDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	ddClient, err := s.ddClient(ch)
	if err != nil {
		return err
	}
	fmt.Printf("ResolvedZone: %s, ResolvedFQDN: %s\n", ch.ResolvedZone, ch.ResolvedFQDN)
	domain := getDomain(ch.ResolvedFQDN)
	subDomain := getSubDomain(domain, ch.ResolvedFQDN)
	target := ch.Key
	return addTXTRecord(ddClient, domain, subDomain, target)
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (s *ddDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	ddClient, err := s.ddClient(ch)
	if err != nil {
		return err
	}
	domain := getDomain(ch.ResolvedFQDN)
	target := ch.Key
	return removeTXTRecord(ddClient, domain, target)
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (s *ddDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	client, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	s.client = client
	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (ddDNSProviderConfig, error) {
	cfg := ddDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding DonDominio config: %v", err)
	}

	return cfg, nil
}

func getDomain(fqdn string) string {
	domain := util.UnFqdn(fqdn)
	i := 0
	n := len(domain)
	for ; n > 0 && i < 2; n-- {
		if domain[n-1] == '.' {
			i++
		}
	}
	if i == 2 {
		return domain[n+1:]
	} else {
		return domain
	}
}

func getSubDomain(domain, fqdn string) string {
	if idx := strings.Index(fqdn, "."+domain); idx != -1 {
		return fqdn[:idx]
	}

	return util.UnFqdn(fqdn)
}

func addTXTRecord(ddClient *Client, domain, subDomain, target string) error {
	err := validateService(ddClient, domain)
	if err != nil {
		return err
	}

	_, err = createRecord(ddClient, domain, "TXT", subDomain, target)

	return err
}

func removeTXTRecord(ddClient *Client, domain, target string) error {
	record, err := findRecords(ddClient, domain, target)
	if err != nil {
		return err
	}

	if record != nil && record.ResponseData.Dns != nil && len(record.ResponseData.Dns) > 0 {
		dns := record.ResponseData.Dns[0]
		err = deleteRecord(ddClient, domain, dns.EntityID)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateService(ddClient *Client, domain string) error {
	url := "/service/getinfo"
	serviceInfo := ddServiceInfo{}
	params := ddServiceStatusParams{
		ServiceName: domain,
		InfoType:    "status",
	}
	err := ddClient.Post(url, &params, &serviceInfo)
	if err != nil {
		return fmt.Errorf("DonDominio API call failed: POST %s - %v", url, err)
	}
	if serviceInfo.ResponseData.Status != "active" {
		return fmt.Errorf("DonDominio service not deployed for domain %s", domain)
	}

	return nil
}

func findRecords(ddClient *Client, domain, target string) (*ddServiceList, error) {
	url := "/service/dnslist"
	serviceList := ddServiceList{}
	params := ddServiceListParams{
		ServiceName: domain,
		FilterValue: target,
	}
	err := ddClient.Post(url, &params, &serviceList)
	if err != nil {
		return nil, fmt.Errorf("DonDominio API call failed: POST %s - %v", url, err)
	}
	return &serviceList, nil
}

func deleteRecord(ddClient *Client, domain, entityId string) error {
	url := "/service/dnsdelete"
	params := ddDeleteServiceParams{
		ServiceName: domain,
		EntityId:    entityId,
	}
	err := ddClient.Post(url, &params, nil)
	if err != nil {
		return fmt.Errorf("DonDominio API call failed: DELETE %s - %v", url, err)
	}
	return nil
}

func createRecord(ddClient *Client, domain, fieldType, subDomain, target string) (*ddServiceList, error) {
	url := "/service/dnscreate"
	params := ddCreateServiceParams{
		FieldType:   fieldType,
		ServiceName: domain,
		Name:        subDomain + "." + domain,
		Value:       target,
	}
	record := ddServiceList{}
	err := ddClient.Post(url, &params, &record)
	if err != nil {
		return nil, fmt.Errorf("DonDOminio API call failed: POST %s - %v", url, err)
	}

	return &record, nil
}
