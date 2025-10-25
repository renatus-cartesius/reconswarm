package provisioning

import (
	"bytes"
	"fmt"
	"text/template"
)

const cloudConfigTemplate = `#cloud-config
ssh_pwauth: no
users:
  - name: {{.Username}}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - "{{.PublicKey}}"`

// CloudConfigData represents the data for cloud-config template
type CloudConfigData struct {
	Username  string
	PublicKey string
}

// GenerateCloudConfig generates cloud-config user-data from template
func GenerateCloudConfig(username, publicKey string) (string, error) {
	tmpl, err := template.New("cloud-config").Parse(cloudConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse cloud-config template: %v", err)
	}

	data := CloudConfigData{
		Username:  username,
		PublicKey: publicKey,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute cloud-config template: %v", err)
	}

	return buf.String(), nil
}
