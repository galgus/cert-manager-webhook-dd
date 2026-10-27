package main

import "testing"

func TestMatchingTXTRecords(t *testing.T) {
	records := []Dns{
		{EntityID: "1", Name: "*.galgus.ai", Type: "CNAME", Value: "parkingsrv0.dondominio.com"},
		{EntityID: "2", Name: "_acme-challenge.pro-ws.galgus.ai", Type: "TXT", Value: "other"},
		{EntityID: "3", Name: "_acme-challenge.pro-ws.galgus.ai", Type: "TXT", Value: "abc"},
		{EntityID: "4", Name: "_acme-challenge.other.galgus.ai", Type: "TXT", Value: "abc"},
		{EntityID: "5", Name: "_ACME-challenge.pro-ws.galgus.ai.", Type: "txt", Value: "\"abc\""},
		{EntityID: "", Name: "_acme-challenge.pro-ws.galgus.ai", Type: "TXT", Value: "abc"},
	}

	got := matchingTXTRecords(records, "_acme-challenge.pro-ws.galgus.ai.", "abc")
	if len(got) != 2 || got[0].EntityID != "3" || got[1].EntityID != "5" {
		t.Fatalf("unexpected matches: %+v", got)
	}

	if got := matchingTXTRecords(records, "_acme-challenge.pro-ws.galgus.ai.", "nomatch"); len(got) != 0 {
		t.Fatalf("nothing must match, got %+v", got)
	}
	if got := matchingTXTRecords(nil, "x.galgus.ai.", "abc"); len(got) != 0 {
		t.Fatalf("empty list must not match, got %+v", got)
	}
}
