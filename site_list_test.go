package main

import (
	"reflect"
	"testing"

	"mikrotool/internal/model"
)

func TestSiteListDefaultsToDescendingNamesWithLetterDividers(t *testing.T) {
	sites := []model.Site{
		{IP: "10.0.0.1", SiteID: "A1", Name: "Alpha"},
		{IP: "10.0.0.2", SiteID: "B1", Name: "Bravo"},
		{IP: "10.0.0.3", SiteID: "A2", Name: "Aardvark"},
	}
	rows := buildSiteListRows(sites, sortBySiteName, false)
	var values []string
	for _, row := range rows {
		if row.divider != "" {
			values = append(values, "divider:"+row.divider)
		} else {
			values = append(values, sites[row.siteIndex].Name)
		}
	}
	want := []string{"divider:B", "Bravo", "divider:A", "Alpha", "Aardvark"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("unexpected default rows: got %v, want %v", values, want)
	}
}

func TestSiteListSortsIPNumericallyInBothDirections(t *testing.T) {
	sites := []model.Site{
		{IP: "10.0.0.10", SiteID: "TEN", Name: "Ten"},
		{IP: "10.0.0.2", SiteID: "TWO", Name: "Two"},
	}
	ascending := buildSiteListRows(sites, sortByIP, true)
	if got := sites[ascending[0].siteIndex].IP; got != "10.0.0.2" {
		t.Fatalf("ascending IP sort started with %q", got)
	}
	descending := buildSiteListRows(sites, sortByIP, false)
	if got := sites[descending[0].siteIndex].IP; got != "10.0.0.10" {
		t.Fatalf("descending IP sort started with %q", got)
	}
}

func TestClosestSiteMatchUsesNamesCodesAndAddresses(t *testing.T) {
	sites := []model.Site{
		{IP: "10.0.0.5", SiteID: "NW-001", Name: "North Warehouse"},
		{IP: "10.0.0.9", SiteID: "SE-002", Name: "South Office"},
	}
	for query, want := range map[string]int{
		"north":       0,
		"soth office": 1,
		"SE-002":      1,
		"10.0.0.5":    0,
	} {
		if got := closestSiteIndex(sites, query); got != want {
			t.Errorf("closestSiteIndex(%q) = %d, want %d", query, got, want)
		}
	}
}

func TestLongOrEmptySearchDoesNotSelect(t *testing.T) {
	sites := []model.Site{{IP: "10.0.0.5", SiteID: "A", Name: "Alpha"}}
	if got := closestSiteIndex(sites, ""); got != -1 {
		t.Fatalf("empty search selected %d", got)
	}
	query := make([]rune, 201)
	for index := range query {
		query[index] = 'a'
	}
	if got := closestSiteIndex(sites, string(query)); got != -1 {
		t.Fatalf("oversized search selected %d", got)
	}
}
