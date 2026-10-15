package check

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// mergeTags returns the union of defaults and explicit tags, defaults first,
// with duplicates removed and stable ordering.
func mergeTags(defaults, explicit []string) []string {
	merged := make([]string, 0, len(defaults)+len(explicit))
	seen := make(map[string]bool, len(defaults)+len(explicit))
	for _, tag := range append(append([]string{}, defaults...), explicit...) {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		merged = append(merged, tag)
	}
	return merged
}

// deriveConfiguredTags splits the practitioner's own tags back out of the
// merged set the API returned, by removing the provider default_tags.
//
// A default the practitioner also listed explicitly must survive, or the next
// plan would show it being removed from "tags" on a check that is correct. The
// prior state is the only record of which tags were written by hand, so a tag
// that is both a default and present in prior "tags" is kept.
//
// Import has no prior state and therefore no way to tell the two apart; there
// the defaults are removed unconditionally, which is the reading that produces
// a configuration the practitioner would have written.
func deriveConfiguredTags(ctx context.Context, all types.Set, defaultTags []string, prior *CheckResourceModel, diags *diag.Diagnostics) types.Set {
	if all.IsNull() || all.IsUnknown() {
		return all
	}

	var merged []string
	diags.Append(all.ElementsAs(ctx, &merged, false)...)
	if diags.HasError() {
		return all
	}

	priorTags := make(map[string]bool)
	havePrior := prior != nil && isSet(prior.Tags)
	if havePrior {
		var written []string
		diags.Append(prior.Tags.ElementsAs(ctx, &written, false)...)
		if diags.HasError() {
			return all
		}
		for _, tag := range written {
			priorTags[tag] = true
		}
	}

	isDefault := make(map[string]bool, len(defaultTags))
	for _, tag := range defaultTags {
		isDefault[tag] = true
	}

	configured := make([]string, 0, len(merged))
	for _, tag := range merged {
		if isDefault[tag] && !priorTags[tag] {
			continue
		}
		configured = append(configured, tag)
	}

	// An absent "tags" argument and an empty one are different configurations,
	// and state has to match whichever the practitioner wrote. With nothing left
	// after the defaults are removed, prior state is what distinguishes them.
	if len(configured) == 0 && !havePrior {
		return types.SetNull(types.StringType)
	}

	result, d := types.SetValueFrom(ctx, types.StringType, configured)
	diags.Append(d...)
	return result
}
