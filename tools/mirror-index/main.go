// Command mirror-index generates the JSON index files for an OpenTofu provider
// network mirror.
//
// The network mirror protocol (https://opentofu.org/docs/internals/provider-network-mirror-protocol/)
// serves two static documents per provider:
//
//	GET <base>/<hostname>/<namespace>/<type>/index.json
//	    {"versions": {"1.0.0": {}, "1.1.0": {}}}
//
//	GET <base>/<hostname>/<namespace>/<type>/<version>.json
//	    {"archives": {"linux_amd64": {"url": "...zip", "hashes": ["h1:..."]}}}
//
// Uploading the zips alone is not sufficient — without these documents OpenTofu
// cannot enumerate or resolve any version.
//
// The "hashes" field is optional in the protocol, and omitting it means the
// archives are installed with NO verification at all. We always emit h1 hashes,
// computed with the same dirhash algorithm OpenTofu uses, so a tampered or
// truncated archive fails the install rather than being silently accepted.
//
// Archive URLs are emitted relative to the version document, so the mirror can
// be rehosted or fronted by a different CDN without regenerating anything.
//
// Usage:
//
//	mirror-index -dist ./dist -version 1.0.0 -out ./mirror \
//	    [-existing-index ./current-index.json]
//
// -existing-index merges previously published versions into the new index.json.
// Without it the index would advertise only the version just built, making every
// earlier release unresolvable.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"
)

type versionIndex struct {
	Versions map[string]struct{} `json:"versions"`
}

type archive struct {
	URL    string   `json:"url"`
	Hashes []string `json:"hashes,omitempty"`
}

type versionDoc struct {
	Archives map[string]archive `json:"archives"`
}

// terraform-provider-<name>_<version>_<os>_<arch>.zip
var archiveRE = regexp.MustCompile(`^terraform-provider-[a-z0-9-]+_([0-9][^_]*)_([a-z0-9]+)_([a-z0-9]+)\.zip$`)

func main() {
	var (
		distDir  = flag.String("dist", "dist", "directory containing the built provider zips")
		version  = flag.String("version", "", "version being published, without a leading v (required)")
		outDir   = flag.String("out", "mirror", "directory to write the index documents into")
		existing = flag.String("existing-index", "", "optional path to the currently published index.json, merged into the output")
	)
	flag.Parse()

	if err := run(*distDir, strings.TrimPrefix(*version, "v"), *outDir, *existing); err != nil {
		fmt.Fprintln(os.Stderr, "mirror-index:", err)
		os.Exit(1)
	}
}

func run(distDir, version, outDir, existingIndex string) error {
	if version == "" {
		return fmt.Errorf("-version is required")
	}

	entries, err := os.ReadDir(distDir)
	if err != nil {
		return fmt.Errorf("reading dist directory: %w", err)
	}

	archives := map[string]archive{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := archiveRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if m[1] != version {
			// A stale archive from an earlier build left in dist/ would
			// otherwise be advertised under the wrong version.
			return fmt.Errorf("archive %s does not match -version %s", e.Name(), version)
		}

		zipPath := filepath.Join(distDir, e.Name())
		// dirhash.HashZip with Hash1 is exactly what OpenTofu computes for an
		// "h1:" provider hash. Reimplementing it risks a subtly wrong value
		// that fails installs with an unhelpful checksum mismatch.
		h, err := dirhash.HashZip(zipPath, dirhash.Hash1)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", e.Name(), err)
		}

		archives[m[2]+"_"+m[3]] = archive{
			URL:    e.Name(), // relative to this version document
			Hashes: []string{h},
		}
	}

	if len(archives) == 0 {
		return fmt.Errorf("no provider archives for version %s found in %s", version, distDir)
	}

	idx := versionIndex{Versions: map[string]struct{}{}}
	if existingIndex != "" {
		raw, err := os.ReadFile(existingIndex)
		switch {
		case err == nil:
			if err := json.Unmarshal(raw, &idx); err != nil {
				return fmt.Errorf("parsing existing index %s: %w", existingIndex, err)
			}
			if idx.Versions == nil {
				idx.Versions = map[string]struct{}{}
			}
		case os.IsNotExist(err):
			// First publish; an absent index is expected.
		default:
			return fmt.Errorf("reading existing index: %w", err)
		}
	}
	idx.Versions[version] = struct{}{}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := writeJSON(filepath.Join(outDir, "index.json"), idx); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, version+".json"), versionDoc{Archives: archives}); err != nil {
		return err
	}

	platforms := make([]string, 0, len(archives))
	for p := range archives {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)

	fmt.Printf("wrote %s/index.json (%d versions)\n", outDir, len(idx.Versions))
	fmt.Printf("wrote %s/%s.json (%d platforms: %s)\n", outDir, version, len(archives), strings.Join(platforms, ", "))
	return nil
}

func writeJSON(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
