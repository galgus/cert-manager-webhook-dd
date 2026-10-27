package main

import (
	"encoding/json"
	"testing"
)

// A dnslist response of DonDominio with ttl and priority as numbers, strings and null
const dnsListResponse = `{
	"success": true,
	"errorCode": 0,
	"action": "service/dnslist",
	"version": "1.0.20",
	"responseData": {
		"queryInfo": {"page": 1, "pageLength": 1000, "results": 3, "total": 3},
		"dns": [
			{"entityID": "SRV1-1", "name": "_acme-challenge.example.com", "type": "TXT", "ttl": 600, "priority": "-", "value": "key"},
			{"entityID": "SRV1-2", "name": "www.example.com", "type": "CNAME", "ttl": "-", "priority": "-", "value": "example.com"},
			{"entityID": "SRV1-3", "name": "example.com", "type": "MX", "ttl": "3600", "priority": 10, "value": "mx.example.com"},
			{"entityID": "SRV1-4", "name": "old.example.com", "type": "A", "ttl": null, "priority": null, "value": "192.0.2.1"}
		]
	}
}`

func TestDnsListWithNumericTtl(t *testing.T) {
	list := ddServiceList{}
	if err := json.Unmarshal([]byte(dnsListResponse), &list); err != nil {
		t.Fatalf("decoding a dnslist response with numeric ttl: %v", err)
	}
	dns := list.ResponseData.Dns
	if len(dns) != 4 {
		t.Fatalf("expected 4 records, got %d", len(dns))
	}
	expected := []struct{ entityID, ttl, priority string }{
		{"SRV1-1", "600", "-"},
		{"SRV1-2", "-", "-"},
		{"SRV1-3", "3600", "10"},
		{"SRV1-4", "", ""},
	}
	for i, e := range expected {
		if dns[i].EntityID != e.entityID || string(dns[i].Ttl) != e.ttl || string(dns[i].Priority) != e.priority {
			t.Errorf("record %d: got %s ttl=%q priority=%q, want %s ttl=%q priority=%q",
				i, dns[i].EntityID, dns[i].Ttl, dns[i].Priority, e.entityID, e.ttl, e.priority)
		}
	}
}

func TestFlexStringRejectsObjects(t *testing.T) {
	var f flexString
	if err := json.Unmarshal([]byte(`{"a":1}`), &f); err == nil {
		t.Fatalf("expected an error decoding an object, got %q", f)
	}
}
