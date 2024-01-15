package service

import (
	"context"
	exporter "github.com/alibaba/kubeskoop/pkg/exporter/cmd"
	"gopkg.in/yaml.v3"
)

func (c *controller) GetExporterConfig(ctx context.Context) (*exporter.InspServerConfig, error) {
	cm, err := c.getConfigMap(ctx, c.ConfigMapNamespace, c.ConfigMapName)
	if err != nil {
		return nil, err
	}
	data := cm.Data["config.yaml"]
	var ret exporter.InspServerConfig
	err = yaml.Unmarshal([]byte(data), &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

func (c *controller) UpdateExporterConfig(ctx context.Context, cfg *exporter.InspServerConfig) error {
	cm, err := c.getConfigMap(ctx, c.ConfigMapNamespace, c.ConfigMapName)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	cm.Data["config.yaml"] = string(data)
	return c.updateConfigMap(ctx, c.ConfigMapNamespace, c.ConfigMapName, cm)
}
