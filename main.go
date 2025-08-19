/*
Copyright 2021 Richard Kosegi

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	//"regexp"
	"strings"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook"
	"k8s.io/klog/v2"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/hostalp/cert-manager-webhook-active24/internal"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type active24DNSProviderSolver struct {
	webhook.Solver
	k8sClient *kubernetes.Clientset
	ctx       context.Context
}

type active24DNSProviderConfig struct {
	ApiKeySecretRef    corev1.SecretKeySelector `json:"apiKeySecretRef"`
	ApiSecretSecretRef corev1.SecretKeySelector `json:"apiSecretSecretRef"`
	ServiceID          int                      `json:"serviceID"`
	Domain             string                   `json:"domain"`
	ApiUrl             string                   `json:"apiUrl"`
	MaxPages           int                      `json:"maxPages"`
}

func main() {
	klog.InitFlags(nil)
	if groupName := os.Getenv("GROUP_NAME"); groupName != "" {
		cmd.RunWebhookServer(groupName, &active24DNSProviderSolver{
			ctx: context.Background(),
		})
	} else {
		panic("GROUP_NAME environment variable must be specified")
	}
}

func (c *active24DNSProviderSolver) Name() string {
	return "active24"
}

func (c *active24DNSProviderSolver) Initialize(restConfig *rest.Config, _ <-chan struct{}) error {
	klog.V(2).Infof("Initialize")

	var err error
	if c.k8sClient, err = kubernetes.NewForConfig(restConfig); err != nil {
		return err
	}
	return nil
}

func (c *active24DNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.V(2).Infof("Present: fqdn=%s, zone=%s, key=%s", ch.ResolvedFQDN, ch.ResolvedZone, ch.Key)

	name, err := c.recordName(ch)
	if err != nil {
		return err
	}

	config, err := clientConfig(c, ch)
	if err != nil {
		return err
	}

	client := internal.NewApiClient(config)
	record, err := client.FindTxtRecordPaged(name, ch.Key)
	if err != nil {
		return err
	}

	klog.V(6).Infof("Record: %v", record)
	if record == nil {
		err := client.NewTxtRecord(name, ch.Key, 300)
		if err != nil {
			return err
		}
	} else {
		err := client.UpdateTxtRecord(*record.ID, name, ch.Key, 300)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *active24DNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	klog.V(2).Infof("CleanUp: zone=%s, fqdn=%s", ch.ResolvedZone, ch.ResolvedFQDN)

	config, err := clientConfig(c, ch)
	if err != nil {
		return err
	}

	name, err := c.recordName(ch)
	if err != nil {
		return err
	}

	client := internal.NewApiClient(config)

	record, err := client.FindTxtRecordPaged(name, ch.Key)
	if err != nil {
		return err
	}

	klog.V(6).Infof("Existing record: %v", record)
	if record != nil {
		return client.DeleteTxtRecord(*record.ID)
	}
	return nil
}

func getK8sSecret(c *active24DNSProviderSolver, ch *v1alpha1.ChallengeRequest, secretName string, secretKey string) ([]byte, error) {
	klog.V(6).Infof("getK8sSecret: Reading secret '%s:%s' in namespace '%s'", secretName, secretKey, ch.ResourceNamespace)
	sec, err := c.k8sClient.CoreV1().Secrets(ch.ResourceNamespace).Get(c.ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get secret `%s/%s`; %v", ch.ResourceNamespace, secretName, err)
	}

	apiKey, ok := sec.Data[secretKey]
	if !ok {
		return nil, fmt.Errorf("key '%q' not found in secret data", secretKey)
	}
	return apiKey, nil
}

func loadConfig(cfgJSON *extapi.JSON) (active24DNSProviderConfig, error) {
	klog.V(6).Infof("loadConfig")
	cfg := active24DNSProviderConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("unable to unmarshal provider config: %v", err)
	}

	return cfg, nil
}

func clientConfig(c *active24DNSProviderSolver, ch *v1alpha1.ChallengeRequest) (internal.Config, error) {
	var config internal.Config

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return config, err
	}
	config.ServiceID = cfg.ServiceID
	config.DomainName = strings.TrimSuffix(ch.ResolvedZone, ".")
	if cfg.Domain != "" {
		config.DomainName = cfg.Domain
	}
	config.ApiUrl = "https://rest.active24.cz"
	if cfg.ApiUrl != "" {
		config.ApiUrl = cfg.ApiUrl
	}
	config.MaxPages = 10 // safeguard to prevent infinite loops
	if cfg.MaxPages > 0 {
		config.MaxPages = cfg.MaxPages
	}

	secretNameApiKey := cfg.ApiKeySecretRef.Name
	secretNameApiSecret := cfg.ApiSecretSecretRef.Name
	secretApiKey := "apiKey"
	secretApiSecret := "apiSecret"
	if cfg.ApiKeySecretRef.Key != "" {
		secretApiKey = cfg.ApiKeySecretRef.Key
	}
	if cfg.ApiSecretSecretRef.Key != "" {
		secretApiSecret = cfg.ApiSecretSecretRef.Key
	}

	// Get API key from the k8s secret
	apiKey, err := getK8sSecret(c, ch, secretNameApiKey, secretApiKey)
	if err != nil {
		return config, err
	}
	config.ApiKey = string(apiKey)

	// Get API secret from the k8s secret
	apiSecret, err := getK8sSecret(c, ch, secretNameApiSecret, secretApiSecret)
	if err != nil {
		return config, err
	}
	config.ApiSecret = string(apiSecret)

	return config, nil
}

// extracts record name from FQDN
func (c *active24DNSProviderSolver) recordName(ch *v1alpha1.ChallengeRequest) (string, error) {
	klog.V(4).Infof("recordName: ResolvedZone=%s, ResolvedFQDN=%s", ch.ResolvedZone, ch.ResolvedFQDN)
	return strings.TrimSuffix(ch.ResolvedFQDN, "."+ch.ResolvedZone), nil
	// Replaced with simple TrimSuffix above
	/* domain := strings.TrimSuffix(ch.ResolvedZone, ".")
	regexStr := "(.+)\\." + domain + "\\."
	r := regexp.MustCompile(regexStr)
	name := r.FindStringSubmatch(ch.ResolvedFQDN)
	if len(name) != 2 {
		return "", fmt.Errorf("unable to extract name from FQDN '%s' using regex '%s'", ch.ResolvedFQDN, regexStr)
	}
	return strings.TrimSuffix(name[1], "."), nil */
}
