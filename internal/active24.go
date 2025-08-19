/*
Copyright 2022 Richard Kosegi

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

package internal

import (
	"fmt"

	"github.com/hostalp/active24-go/active24"
	"k8s.io/klog/v2"
)

type Config struct {
	ApiKey     string
	ApiSecret  string
	ApiUrl     string
	DomainName string
	ServiceID  int
	MaxPages   int
}

type ApiClient struct {
	dns      active24.DnsRecordActions
	dom      string
	svcID    int
	maxPages int
}

// FindTxtRecord Find TXT record by name and content
func (a *ApiClient) FindTxtRecord(name string, content string) (*active24.DnsRecord, error) {
	klog.V(4).Infof("FindTxtRecord: domain=%s, service=%d, name=%s, content=%s",
		a.dom, a.svcID, name, content)

	records, err := a.dns.List(active24.DnsRecordTypeTXT, name)
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return nil, err.Error()
	}
	if klog.V(9).Enabled() {
		klog.V(9).Infof("records=%v", records)
	}
	for _, record := range records {
		if klog.V(9).Enabled() {
			klog.V(9).Infof("record=%v, content=%s", record, *record.Content)
		}
		if record.Name == name+"."+a.dom && *record.Content == content {
			klog.V(4).Infof("Found record ID: %d", *record.ID)
			return &record, nil
		}
	}
	klog.V(4).Infof("Didn't find a record")
	return nil, nil
}

// FindTxtRecordPaged Find TXT record by name and content with pagination support
func (a *ApiClient) FindTxtRecordPaged(name string, content string) (*active24.DnsRecord, error) {
	klog.V(4).Infof("FindTxtRecordPaged: domain=%s, service=%d, name=%s, content=%s",
		a.dom, a.svcID, name, content)

	// Get and process first page
	record, nextPageUrl, nextPage, err := a.FindTxtRecordAtPage(name, content, "", 0)
	if err != nil {
		klog.V(1).ErrorS(err, "error finding a record")
		return nil, err
	}
	if record != nil {
		return record, nil
	}

	// Keep getting and processing next pages while we have either nextPageUrl or nextPage
	pageCount := 1
	for (nextPageUrl != "" || nextPage > 0) && pageCount < a.maxPages {
		pageCount++
		record, nextPageUrl, nextPage, err = a.FindTxtRecordAtPage(name, content, nextPageUrl, nextPage)
		if err != nil {
			klog.V(1).ErrorS(err, "error finding a record")
			return nil, err
		}
		if record != nil {
			return record, nil
		}
	}
	if pageCount >= a.maxPages {
		err := fmt.Errorf("maximum page limit %d reached in FindTxtRecordPaged, increase the MaxPages limit in the ClusterIssuer configuration", a.maxPages)
		klog.V(1).ErrorS(err, "maxPages", a.maxPages)
		return nil, err
	}
	return nil, nil
}

// FindTxtRecordAtPage Find TXT record by name and content at a specific page
func (a *ApiClient) FindTxtRecordAtPage(name string, content string, recPageUrl string, recPage int) (*active24.DnsRecord, string, int, error) {
	klog.V(4).Infof("FindTxtRecordAtPage: domain=%s, service=%d, name=%s, content=%s, pageUrl=%s or page=%d",
		a.dom, a.svcID, name, content, recPageUrl, recPage)

	records, nextPageUrl, nextPage, err := a.dns.ListPage(active24.DnsRecordTypeTXT, name, recPageUrl, recPage)
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return nil, "", 0, err.Error()
	}
	if klog.V(9).Enabled() {
		klog.V(9).Infof("records=%v", records)
	}
	for _, record := range records {
		if klog.V(9).Enabled() {
			klog.V(9).Infof("record=%v, content=%s, nextPageUrl=%s or nextPage=%d", record, *record.Content, nextPageUrl, nextPage)
		}
		if record.Name == name+"."+a.dom && *record.Content == content {
			klog.V(4).Infof("Found record ID: %d", *record.ID)
			return &record, nextPageUrl, nextPage, nil
		}
	}
	klog.V(4).Infof("Didn't find a record")
	return nil, nextPageUrl, nextPage, nil
}

// NewTxtRecord Create new DNS TXT record
func (a *ApiClient) NewTxtRecord(name string, content string, ttl int) error {
	klog.V(4).Infof("NewTxtRecord: domain=%s, service=%d, name=%s, content=%s, ttl=%d",
		a.dom, a.svcID, name, content, ttl)
	rtype := string(active24.DnsRecordTypeTXT)
	err := a.dns.Create(&active24.DnsRecord{
		Type:    &rtype,
		Name:    name,
		Content: &content,
		Ttl:     ttl,
	})
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return err.Error()
	}
	return nil
}

// UpdateTxtRecord Update existing DNS TXT record
func (a *ApiClient) UpdateTxtRecord(ID int, name string, content string, ttl int) error {
	klog.V(4).Infof("UpdateTxtRecord: domain=%s, service=%d, name=%s, content=%s, ttl=%d, ID=%d",
		a.dom, a.svcID, name, content, ttl, ID)
	err := a.dns.Update(ID, &active24.DnsRecord{
		Name:    name,
		Content: &content,
		Ttl:     ttl,
	})
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return err.Error()
	}
	return nil
}

// DeleteTxtRecord Delete existing DNS record
func (a *ApiClient) DeleteTxtRecord(ID int) error {
	klog.V(4).Infof("DeleteTxtRecord: domain=%s, service=%d, ID=%d", a.dom, a.svcID, ID)
	err := a.dns.Delete(ID)
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return err.Error()
	}
	return nil
}

func NewApiClient(config Config) *ApiClient {
	opts := make([]active24.Option, 0)
	if len(config.ApiUrl) > 0 {
		opts = append(opts, active24.ApiEndpoint(config.ApiUrl))
	}
	return &ApiClient{
		dns:      active24.New(config.ApiKey, config.ApiSecret, opts...).Dns().With(config.DomainName, config.ServiceID),
		dom:      config.DomainName,
		svcID:    config.ServiceID,
		maxPages: config.MaxPages,
	}
}
