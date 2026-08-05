package main

import (
	"net/netip"
	"sort"
	"strings"
	"unicode"

	"mikrotool/internal/model"
)

type siteSortField uint8

const (
	sortByIP siteSortField = iota
	sortByCompanyCode
	sortBySiteName
)

type siteListRow struct {
	divider   string
	siteIndex int
}

func buildSiteListRows(sites []model.Site, field siteSortField, ascending bool) []siteListRow {
	indices := make([]int, len(sites))
	for index := range sites {
		indices[index] = index
	}
	sort.SliceStable(indices, func(i, j int) bool {
		comparison := compareSites(sites[indices[i]], sites[indices[j]], field)
		if !ascending {
			comparison = -comparison
		}
		return comparison < 0
	})

	rows := make([]siteListRow, 0, len(indices)*2)
	previousDivider := ""
	for _, siteIndex := range indices {
		if field == sortBySiteName {
			divider := siteNameDivider(sites[siteIndex].Name)
			if divider != previousDivider {
				rows = append(rows, siteListRow{divider: divider, siteIndex: -1})
				previousDivider = divider
			}
		}
		rows = append(rows, siteListRow{siteIndex: siteIndex})
	}
	return rows
}

func compareSites(left, right model.Site, field siteSortField) int {
	var comparison int
	switch field {
	case sortByIP:
		comparison = compareIP(left.IP, right.IP)
	case sortByCompanyCode:
		comparison = strings.Compare(strings.ToLower(left.CompanyCode), strings.ToLower(right.CompanyCode))
	default:
		comparison = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	}
	if comparison != 0 {
		return comparison
	}
	if comparison = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name)); comparison != 0 {
		return comparison
	}
	if comparison = strings.Compare(strings.ToLower(left.CompanyCode), strings.ToLower(right.CompanyCode)); comparison != 0 {
		return comparison
	}
	return compareIP(left.IP, right.IP)
}

func compareIP(left, right string) int {
	leftAddress, leftErr := netip.ParseAddr(strings.TrimSpace(left))
	rightAddress, rightErr := netip.ParseAddr(strings.TrimSpace(right))
	if leftErr == nil && rightErr == nil {
		return leftAddress.Compare(rightAddress)
	}
	return strings.Compare(left, right)
}

func siteNameDivider(name string) string {
	for _, character := range strings.TrimSpace(name) {
		if unicode.IsLetter(character) {
			return strings.ToUpper(string(character))
		}
		return "#"
	}
	return "#"
}

func closestSiteIndex(sites []model.Site, query string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || len([]rune(query)) > 200 {
		return -1
	}
	bestIndex := -1
	bestScore := int(^uint(0) >> 1)
	for index, site := range sites {
		candidates := []string{
			strings.ToLower(site.Name),
			strings.ToLower(site.CompanyCode),
			strings.ToLower(site.IP),
		}
		for fieldIndex, candidate := range candidates {
			score := matchScore(candidate, query) + fieldIndex*10
			if score < bestScore {
				bestIndex = index
				bestScore = score
			}
		}
	}
	return bestIndex
}

func matchScore(candidate, query string) int {
	switch {
	case candidate == query:
		return 0
	case strings.HasPrefix(candidate, query):
		return 100 + len([]rune(candidate)) - len([]rune(query))
	case strings.Contains(candidate, query):
		return 1_000 + strings.Index(candidate, query)
	default:
		return 10_000 + editDistance(candidate, query)
	}
}

func editDistance(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range rightRunes {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			current[rightIndex+1] = min(
				current[rightIndex]+1,
				previous[rightIndex+1]+1,
				previous[rightIndex]+cost,
			)
		}
		previous = current
	}
	return previous[len(rightRunes)]
}
