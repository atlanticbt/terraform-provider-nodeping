package checks

import (
	"reflect"
	"testing"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

func TestCheckFilterMatches(t *testing.T) {
	check := client.Check{Type: "HTTP", Label: "web", Tags: []string{"prod", "edge"}}

	cases := []struct {
		name   string
		filter checkFilter
		want   bool
	}{
		{"no filters", checkFilter{}, true},
		{"type is case-insensitive", checkFilter{checkType: "http"}, true},
		{"type mismatch", checkFilter{checkType: "PING"}, false},
		{"label exact", checkFilter{label: "web"}, true},
		{"label is case-sensitive", checkFilter{label: "Web"}, false},
		{"all tags present", checkFilter{tags: []string{"prod"}}, true},
		{"a tag missing", checkFilter{tags: []string{"prod", "staging"}}, false},
	}
	for _, c := range cases {
		if got := c.filter.matches(check); got != c.want {
			t.Errorf("%s: matches = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSortedCheckIDs(t *testing.T) {
	got := sortedCheckIDs(map[string]client.Check{"b": {}, "a": {}, "c": {}})
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
