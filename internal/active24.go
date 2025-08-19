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
	ApiKey    string
	ApiSecret string
	ApiUrl    string
	ServiceID int
	MaxPages  int
}

type ApiClient struct {
	dns      active24.DnsRecordActions
	svcID    int
	maxPages int
}

// FindTxtRecord Find TXT record by name and content
func (a *ApiClient) FindTxtRecord(domName string, recName string, content string) (*active24.DnsRecord, error) {
	klog.V(4).Infof("FindTxtRecord: serviceID=%d, domain=%s, name=%s, content=%s",
		a.svcID, domName, recName, content)

	records, err := a.dns.List(active24.DnsRecordTypeTXT, recName)
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return nil, err.Error()
	}
	if klog.V(9).Enabled() {
		klog.V(9).Infof("records=%v", records)
	}
	for i := range records {
		if klog.V(9).Enabled() {
			klog.V(9).Infof("record=%v, content=%s", records[i], *records[i].Content)
		}
		if records[i].Name == recName+"."+domName && *records[i].Content == content {
			klog.V(4).Infof("Found record ID: %d", *records[i].ID)
			return &records[i], nil
		}
	}
	klog.V(4).Infof("Didn't find a record")
	return nil, nil
}

// FindTxtRecordPaged Find TXT record by name and content with pagination support
func (a *ApiClient) FindTxtRecordPaged(domName string, recName string, content string) (*active24.DnsRecord, error) {
	klog.V(4).Infof("FindTxtRecordPaged: serviceID=%d, domain=%s, name=%s, content=%s",
		a.svcID, domName, recName, content)

	var record *active24.DnsRecord
	var nextPageUrl string
	var nextPage int
	var err error

	pageCount := 1
	for (pageCount == 1 || nextPageUrl != "" || nextPage > 0) && pageCount <= a.maxPages {
		record, nextPageUrl, nextPage, err = a.FindTxtRecordAtPage(domName, recName, content, nextPageUrl, nextPage)
		if err != nil {
			klog.V(1).ErrorS(err, "error finding a record")
			return nil, err
		}
		if record != nil {
			return record, nil
		}
		pageCount++
	}
	if pageCount >= a.maxPages && (nextPageUrl != "" || nextPage > a.maxPages) {
		err = fmt.Errorf("maximum page limit %d reached in FindTxtRecordPaged, increase the MaxPages limit in the ClusterIssuer configuration", a.maxPages)
		klog.V(1).ErrorS(err, "maxPages", a.maxPages)
		return nil, err
	}
	return nil, nil
}

// FindTxtRecordAtPage Find TXT record by name and content at a specific page
func (a *ApiClient) FindTxtRecordAtPage(domName string, recName string, content string, recPageUrl string, recPage int) (*active24.DnsRecord, string, int, error) {
	klog.V(4).Infof("FindTxtRecordAtPage: serviceID=%d, domain=%s, name=%s, content=%s, pageUrl=%s or page=%d",
		a.svcID, domName, recName, content, recPageUrl, recPage)

	records, nextPageUrl, nextPage, err := a.dns.ListPage(active24.DnsRecordTypeTXT, recName, recPageUrl, recPage)
	if err != nil {
		klog.V(1).ErrorS(err.Error(), "invalid API response", "code", err.Response().Status)
		return nil, "", 0, err.Error()
	}
	if klog.V(9).Enabled() {
		klog.V(9).Infof("records=%v", records)
	}
	for i := range records {
		if klog.V(9).Enabled() {
			klog.V(9).Infof("record=%v, content=%s, nextPageUrl=%s or nextPage=%d", records[i], *records[i].Content, nextPageUrl, nextPage)
		}
		if records[i].Name == recName+"."+domName && *records[i].Content == content {
			klog.V(4).Infof("Found record ID: %d", *records[i].ID)
			return &records[i], nextPageUrl, nextPage, nil
		}
	}
	klog.V(4).Infof("Didn't find a record")
	return nil, nextPageUrl, nextPage, nil
}

// NewTxtRecord Create new DNS TXT record
func (a *ApiClient) NewTxtRecord(recName string, content string, ttl int) error {
	klog.V(4).Infof("NewTxtRecord: serviceID=%d, name=%s, content=%s, ttl=%d",
		a.svcID, recName, content, ttl)
	rtype := string(active24.DnsRecordTypeTXT)
	err := a.dns.Create(&active24.DnsRecord{
		Type:    &rtype,
		Name:    recName,
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
func (a *ApiClient) UpdateTxtRecord(ID int, recName string, content string, ttl int) error {
	klog.V(4).Infof("UpdateTxtRecord: serviceID=%d, name=%s, content=%s, ttl=%d, ID=%d",
		a.svcID, recName, content, ttl, ID)
	err := a.dns.Update(ID, &active24.DnsRecord{
		Name:    recName,
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
	klog.V(4).Infof("DeleteTxtRecord: serviceID=%d, ID=%d", a.svcID, ID)
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
		dns:      active24.New(config.ApiKey, config.ApiSecret, opts...).Dns().With(config.ServiceID),
		svcID:    config.ServiceID,
		maxPages: config.MaxPages,
	}
}
