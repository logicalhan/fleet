package patch

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/pkg/errors"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/pkg/content"
	"github.com/rancher/fleet/pkg/manifest"
	"github.com/rancher/wrangler/pkg/patch"
)

func Process(m *manifest.Manifest) (*manifest.Manifest, error) {
	newContent, err := decodeContext(m)
	if err != nil {
		return nil, err
	}

	newManifest := &manifest.Manifest{}
	for name, content := range newContent {
		newManifest.Resources = append(newManifest.Resources, fleet.BundleResource{
			Name:    name,
			Content: string(content),
		})
	}

	sort.Slice(newManifest.Resources, func(i, j int) bool {
		return newManifest.Resources[i].Name < newManifest.Resources[j].Name
	})

	return newManifest, nil
}

func decodeContext(m *manifest.Manifest) (map[string][]byte, error) {
	result := map[string][]byte{}

	for i, resource := range m.Resources {
		name := resource.Name
		if name == "" {
			name = fmt.Sprintf("manifests/file%03d", i)
		}

		data, err := content.Decode(resource.Content, resource.Encoding)
		if err != nil {
			return nil, err
		}

		result[name] = data
	}

	if err := patchContent(result); err != nil {
		return nil, err
	}

	return result, nil
}

func isPatchFile(name string) (string, bool) {
	base := filepath.Base(name)
	if strings.Contains(base, "_patch.") {
		target := strings.Replace(base, "_patch.", ".", 1)
		return filepath.Join(filepath.Dir(name), target), true
	}
	return "", false
}

func patchContent(content map[string][]byte) error {
	for name, bytes := range content {
		target, ok := isPatchFile(name)
		if !ok {
			continue
		}
		delete(content, name)

		targetContent, ok := content[target]
		if !ok {
			return fmt.Errorf("failed to find base file %s to patch", target)
		}

		targetContent, err := convertToJSON(targetContent)
		if err != nil {
			return errors.Wrapf(err, "failed to convert %s to json", target)
		}

		bytes, err = convertToJSON(bytes)
		if err != nil {
			return errors.Wrapf(err, "failed to convert %s to json", name)
		}

		newBytes, err := patch.Apply(targetContent, bytes)
		if err != nil {
			return errors.Wrapf(err, "failed to patch %s", target)
		}
		content[target] = newBytes
	}

	return nil
}

func convertToJSON(input []byte) ([]byte, error) {
	var data interface{}
	data = map[string]interface{}{}
	if err := yaml.Unmarshal(input, &data); err != nil {
		data = []interface{}{}
		if err := yaml.Unmarshal(input, &data); err != nil {
			return nil, err
		}
	}
	return json.Marshal(data)
}
