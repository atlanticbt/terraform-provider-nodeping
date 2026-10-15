package check

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

// NodePing only ever reports the merged set, so Read has to split the
// practitioner's own tags back out of it. Getting this wrong in either
// direction produces a plan that never settles: keep too much and "tags" stops
// matching the configuration, drop too much and a tag the practitioner wrote is
// planned for removal on every run.
func TestDeriveConfiguredTags(t *testing.T) {
	ctx := testContext()

	set := func(values ...string) types.Set {
		return mustSet(t, ctx, values...)
	}
	priorWith := func(tags types.Set) *CheckResourceModel {
		return &CheckResourceModel{Tags: tags}
	}

	cases := []struct {
		name     string
		all      types.Set
		defaults []string
		prior    *CheckResourceModel
		want     []string
		wantNull bool
	}{
		{
			name:     "defaults are removed from the merged set",
			all:      set("managed", "prod"),
			defaults: []string{"managed"},
			prior:    priorWith(set("prod")),
			want:     []string{"prod"},
		},
		{
			name:     "a default the practitioner also wrote is kept",
			all:      set("managed", "prod"),
			defaults: []string{"managed"},
			prior:    priorWith(set("managed", "prod")),
			want:     []string{"managed", "prod"},
		},
		{
			name:     "a tag added out of band shows as drift",
			all:      set("managed", "prod", "rogue"),
			defaults: []string{"managed"},
			prior:    priorWith(set("prod")),
			want:     []string{"prod", "rogue"},
		},
		{
			name:     "a tag removed out of band shows as drift",
			all:      set("managed"),
			defaults: []string{"managed"},
			prior:    priorWith(set("prod")),
			want:     []string{},
		},
		{
			name:     "a default dropped from the provider becomes a configured tag",
			all:      set("managed", "prod"),
			defaults: nil,
			prior:    priorWith(set("prod")),
			want:     []string{"managed", "prod"},
		},
		{
			name:     "no defaults leaves the merged set untouched",
			all:      set("prod"),
			defaults: nil,
			prior:    priorWith(set("prod")),
			want:     []string{"prod"},
		},
		{
			name:     "an absent tags argument stays absent when only defaults are applied",
			all:      set("managed"),
			defaults: []string{"managed"},
			prior:    priorWith(types.SetNull(types.StringType)),
			wantNull: true,
		},
		{
			name:     "an empty tags argument stays empty rather than becoming absent",
			all:      set("managed"),
			defaults: []string{"managed"},
			prior:    priorWith(emptyTagSet()),
			want:     []string{},
		},
		{
			name:     "import with no prior state removes the defaults",
			all:      set("managed", "prod"),
			defaults: []string{"managed"},
			prior:    nil,
			want:     []string{"prod"},
		},
		{
			name:     "import of a check carrying only defaults yields no tags argument",
			all:      set("managed"),
			defaults: []string{"managed"},
			prior:    nil,
			wantNull: true,
		},
		{
			name:     "a check with no tags at all stays null",
			all:      types.SetNull(types.StringType),
			defaults: []string{"managed"},
			prior:    priorWith(set("prod")),
			wantNull: true,
		},
		{
			name:     "an unknown merged set is passed through",
			all:      types.SetUnknown(types.StringType),
			defaults: []string{"managed"},
			prior:    nil,
			wantNull: false,
			want:     nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := deriveConfiguredTags(ctx, tc.all, tc.defaults, tc.prior, &diags)
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}

			if tc.all.IsUnknown() {
				if !got.IsUnknown() {
					t.Fatalf("got = %v, want unknown", got)
				}
				return
			}
			if tc.wantNull {
				if !got.IsNull() {
					t.Fatalf("got = %v, want null", got)
				}
				return
			}
			if got.IsNull() || got.IsUnknown() {
				t.Fatalf("got = %v, want %v", got, tc.want)
			}

			have := tagStrings(t, got)
			if len(have) != len(tc.want) {
				t.Fatalf("got = %v, want %v", have, tc.want)
			}
			wanted := make(map[string]bool, len(tc.want))
			for _, tag := range tc.want {
				wanted[tag] = true
			}
			for _, tag := range have {
				if !wanted[tag] {
					t.Fatalf("got = %v, want %v", have, tc.want)
				}
			}
		})
	}
}

// The round trip is what the practitioner sees: a check created with default
// tags applied must read back with "tags" still matching the configuration and
// "tags_all" holding what NodePing stores.
func TestMapForRead_SplitsDefaultTagsOutOfTheMergedSet(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	check := &client.Check{
		ID:   "CHK-1",
		Type: "HTTP",
		Tags: []string{"managed-by-tofu", "team-devops", "tf-acc"},
	}
	defaults := []string{"managed-by-tofu", "tf-acc"}

	prior := &CheckResourceModel{Tags: mustSet(t, ctx, "team-devops")}
	refreshed := mapForRead(ctx, check, prior, defaults, &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got := tagStrings(t, refreshed.Tags); len(got) != 1 || got[0] != "team-devops" {
		t.Errorf("tags = %v, want only the configured team-devops", got)
	}
	if got := tagStrings(t, refreshed.TagsAll); len(got) != 3 {
		t.Errorf("tags_all = %v, want all three", got)
	}

	imported := mapForRead(ctx, check, nil, defaults, &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got := tagStrings(t, imported.Tags); len(got) != 1 || got[0] != "team-devops" {
		t.Errorf("imported tags = %v, want only team-devops", got)
	}
	if got := tagStrings(t, imported.TagsAll); len(got) != 3 {
		t.Errorf("imported tags_all = %v, want all three", got)
	}
}

// emptyTagSet is a known set with no elements. mustSet cannot express it:
// called with no values it receives a nil slice and returns a null set, and an
// absent "tags" argument and an empty one are different configurations.
func emptyTagSet() types.Set {
	return types.SetValueMust(types.StringType, []attr.Value{})
}
