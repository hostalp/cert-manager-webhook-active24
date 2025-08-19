# ACME webhook for Active24 DNS APIv2

This repository contains code and supporting files for ACME webhook that interacts with [active24.cz](https://rest.active24.cz/v2/docs) DNS APIv2.

## Installation

### Requirements

- [cert-manager](https://cert-manager.io/docs/installation/)

- [API key and secret](https://admin.active24.cz/en/auth/security-settings) to access your domain

- [Service ID](https://admin.active24.cz/en/services) to be determined from the link to the desired service (domain), example: `12345678` for `https://admin.active24.cz/en/dashboard/service/12345678`

Create secret with API key and secret

```
kubectl create secret generic active24-apikey --namespace cert-manager \
	--from-literal='apiKey=abcd1234567890' --from-literal='apiSecret=defg0987654321'
```

Create ClusterIssuer


Apply the following manifest into cluster

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    # The ACME server URL
    server: https://acme-v02.api.letsencrypt.org/directory
    # Email address used for ACME registration
    email: admin@somegreatdomain.tld
    # Name of a secret used to store the ACME account private key
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - selector:
        dnsZones:
          - somegreatdomain.tld
      dns01:
        webhook:
          groupName: acme.yourdomain.tld # apiGroup from cert-manager-webhook-active24 Helm chart
          solverName: active24
          config:
            apiKeySecretRef:
              name: &apiKSName 'active24-apikey'
              key: 'apiKey'
            apiSecretSecretRef:
              name: *apiKSName
              key: 'apiSecret'
            serviceID: 12345678
            domain: somegreatdomain.tld # optional
            maxPages: 10 # optional
```

`domain` is optional. If not set, it will be determined from [cert-manager ChallengeRequest.ResolvedZone](https://github.com/cert-manager/cert-manager/blob/master/pkg/acme/webhook/apis/acme/v1alpha1/types.go).
If setting explicitly, specify the actual domain managed by Active24.

`maxPages` is optional. It specifies page limit for paginated DNS records that Active24 DNS APIv2 returns. Default value is 100.
Default page size (currently not modified by this webhook) is 20 e.g. this webhook will handle situations with up to 2000 _acme-challenge DNS TXT records by default.

Install using helm

```
helm upgrade --install ac24 ./chart --namespace cert-manager
```

Create certificate

```yaml
kind: Certificate
apiVersion: cert-manager.io/v1
metadata:
  name: &certName my-certificate
spec:
  commonName: &commonName somegreatdomain.tld
  dnsNames:
    - *commonName
    - '*.somegreatdomain.tld'
  issuerRef:
    kind: ClusterIssuer
    name: letsencrypt-prod
  secretName: *certName
```
