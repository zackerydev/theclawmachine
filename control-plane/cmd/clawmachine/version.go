package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
	"helm.sh/helm/v4/pkg/chart/loader"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

type botChartSpec struct {
	name string
	kind service.BotType
}

type vendoredChartSpec struct {
	getter func() []byte
}

var botChartSpecs = []botChartSpec{
	{name: "openclaw", kind: service.BotTypeOpenClaw},
	{name: "picoclaw", kind: service.BotTypePicoClaw},
	{name: "ironclaw", kind: service.BotTypeIronClaw},
	{name: "busybox", kind: service.BotTypeBusyBox},
}

var vendoredChartSpecs = []vendoredChartSpec{
	{getter: service.GetESOChart},
	{getter: service.GetCiliumChart},
	{getter: service.GetConnectChart},
}

func newVersionCmd() *cobra.Command {
	var showAll bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the ClawMachine version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(cmd.OutOrStdout(), showAll)
		},
	}
	cmd.Flags().BoolVar(&showAll, "all", false, "Print CLI version plus canonical bot image refs and vendored chart checksums")
	return cmd
}

func runVersion(w io.Writer, showAll bool) error {
	if _, err := fmt.Fprintln(w, titleStyle.Render("clawmachine")+" "+accentStyle.Render("v"+version)); err != nil {
		return fmt.Errorf("writing version output: %w", err)
	}
	if !showAll {
		return nil
	}

	botImages, err := resolveBotImageRefs()
	if err != nil {
		return err
	}
	vendoredCharts, err := resolveVendoredChartChecksums()
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("writing bot image section spacing: %w", err)
	}
	if _, err := fmt.Fprintln(w, "bot images (canonical repo:tag):"); err != nil {
		return fmt.Errorf("writing bot image section header: %w", err)
	}
	for _, bot := range botImages {
		if _, err := fmt.Fprintf(w, "  - %s: %s:%s\n", bot.name, bot.repository, bot.tag); err != nil {
			return fmt.Errorf("writing bot image ref for %s: %w", bot.name, err)
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("writing vendored chart section spacing: %w", err)
	}
	if _, err := fmt.Fprintln(w, "vendored charts (sha256):"); err != nil {
		return fmt.Errorf("writing vendored chart section header: %w", err)
	}
	for _, chart := range vendoredCharts {
		if _, err := fmt.Fprintf(w, "  - %s@%s: %s\n", chart.name, chart.version, chart.sha256); err != nil {
			return fmt.Errorf("writing vendored chart checksum for %s: %w", chart.name, err)
		}
	}

	return nil
}

type botImageRef struct {
	name       string
	repository string
	tag        string
}

func resolveBotImageRefs() ([]botImageRef, error) {
	refs := make([]botImageRef, 0, len(botChartSpecs))
	for _, spec := range botChartSpecs {
		archive, err := service.GetEmbeddedChart(spec.kind)
		if err != nil {
			return nil, fmt.Errorf("resolving embedded %s chart: %w", spec.name, err)
		}

		chrt, err := loader.LoadArchive(bytes.NewReader(archive))
		if err != nil {
			return nil, fmt.Errorf("loading embedded %s chart: %w", spec.name, err)
		}
		chrtV2, err := asV2Chart(chrt)
		if err != nil {
			return nil, fmt.Errorf("accessing embedded %s chart: %w", spec.name, err)
		}

		repo, tag, err := imageRefFromValues(chrtV2.Values)
		if err != nil {
			return nil, fmt.Errorf("resolving image ref for %s: %w", spec.name, err)
		}
		refs = append(refs, botImageRef{name: spec.name, repository: repo, tag: tag})
	}
	return refs, nil
}

type vendoredChartChecksum struct {
	name    string
	version string
	sha256  string
}

func resolveVendoredChartChecksums() ([]vendoredChartChecksum, error) {
	checksums := make([]vendoredChartChecksum, 0, len(vendoredChartSpecs))
	for _, spec := range vendoredChartSpecs {
		archive := spec.getter()
		chrt, err := loader.LoadArchive(bytes.NewReader(archive))
		if err != nil {
			return nil, fmt.Errorf("loading embedded vendored chart: %w", err)
		}
		chrtV2, err := asV2Chart(chrt)
		if err != nil {
			return nil, fmt.Errorf("accessing embedded vendored chart: %w", err)
		}

		name := strings.TrimSpace(chrtV2.Name())
		version := ""
		if chrtV2.Metadata != nil {
			version = strings.TrimSpace(chrtV2.Metadata.Version)
		}
		if name == "" || version == "" {
			return nil, fmt.Errorf("vendored chart metadata is incomplete")
		}

		sum := sha256.Sum256(archive)
		checksums = append(checksums, vendoredChartChecksum{
			name:    name,
			version: version,
			sha256:  "sha256:" + fmt.Sprintf("%x", sum[:]),
		})
	}
	return checksums, nil
}

func imageRefFromValues(values map[string]any) (repository, tag string, err error) {
	if values == nil {
		return "", "", fmt.Errorf("chart values are empty")
	}
	imageAny, ok := values["image"]
	if !ok {
		return "", "", fmt.Errorf("image values are missing")
	}
	imageMap, ok := imageAny.(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("image values have unexpected type %T", imageAny)
	}

	repoAny, ok := imageMap["repository"]
	if !ok {
		return "", "", fmt.Errorf("image.repository is missing")
	}
	tagAny, ok := imageMap["tag"]
	if !ok {
		return "", "", fmt.Errorf("image.tag is missing")
	}

	repository = strings.TrimSpace(fmt.Sprintf("%v", repoAny))
	tag = strings.TrimSpace(fmt.Sprintf("%v", tagAny))
	if repository == "" || tag == "" {
		return "", "", fmt.Errorf("image.repository or image.tag is empty")
	}
	return repository, tag, nil
}

func asV2Chart(chrt any) (*chartv2.Chart, error) {
	chrtV2, ok := chrt.(*chartv2.Chart)
	if !ok || chrtV2 == nil {
		return nil, fmt.Errorf("unexpected chart type %T", chrt)
	}
	return chrtV2, nil
}
