// Package azuredns implements a DNS provider for solving the DNS-01 challenge using azure DNS.
// Azure doesn't like trailing dots on domain names, most of the acme code does.
package azuredns

import (
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/platform/config/env"
)

// Environment variables names.
const (
	envNamespace = "AZURE_"

	EnvEnvironment    = envNamespace + "ENVIRONMENT"
	EnvSubscriptionID = envNamespace + "SUBSCRIPTION_ID"
	EnvResourceGroup  = envNamespace + "RESOURCE_GROUP"
	EnvZoneName       = envNamespace + "ZONE_NAME"
	EnvPrivateZone    = envNamespace + "PRIVATE_ZONE"

	EnvTTL                = envNamespace + "TTL"
	EnvPropagationTimeout = envNamespace + "PROPAGATION_TIMEOUT"
	EnvPollingInterval    = envNamespace + "POLLING_INTERVAL"
)

// Config is used to configure the creation of the DNSProvider.
type Config struct {
	SubscriptionID string
	ResourceGroup  string
	PrivateZone    bool

	Environment cloud.Configuration

	PropagationTimeout time.Duration
	PollingInterval    time.Duration
	TTL                int
}

// NewDefaultConfig returns a default configuration for the DNSProvider.
func NewDefaultConfig() *Config {
	return &Config{
		TTL:                env.GetOrDefaultInt(EnvTTL, 60),
		PropagationTimeout: env.GetOrDefaultSecond(EnvPropagationTimeout, 2*time.Minute),
		PollingInterval:    env.GetOrDefaultSecond(EnvPollingInterval, 2*time.Second),
		Environment:        cloud.AzurePublic,
	}
}

// DNSProvider implements the challenge.Provider interface.
type DNSProvider struct {
	provider challenge.ProviderTimeout
}

// NewDNSProvider returns a DNSProvider instance configured for azuredns.
func NewDNSProvider() (*DNSProvider, error) {
	config := NewDefaultConfig()

	environmentName := env.GetOrFile(EnvEnvironment)
	if environmentName != "" {
		switch environmentName {
		case "china":
			config.Environment = cloud.AzureChina
		case "public":
			config.Environment = cloud.AzurePublic
		case "usgovernment":
			config.Environment = cloud.AzureGovernment
		default:
			return nil, fmt.Errorf("azuredns: unknown environment %s", environmentName)
		}
	}

	config.SubscriptionID = env.GetOrFile(EnvSubscriptionID)
	config.ResourceGroup = env.GetOrFile(EnvResourceGroup)
	config.PrivateZone = env.GetOrDefaultBool(EnvPrivateZone, false)

	return NewDNSProviderConfig(config)
}

// NewDNSProviderConfig return a DNSProvider instance configured for Azure.
func NewDNSProviderConfig(config *Config) (*DNSProvider, error) {
	if config == nil {
		return nil, errors.New("azuredns: the configuration of the DNS provider is nil")
	}

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azuredns: %w", err)
	}

	if config.SubscriptionID == "" {
		return nil, errors.New("azuredns: SubscriptionID is missing")
	}

	if config.ResourceGroup == "" {
		return nil, errors.New("azuredns: ResourceGroup is missing")
	}

	var dnsProvider challenge.Provider
	if config.PrivateZone {
		dnsProvider, err = NewDNSProviderPrivate(config, credentials)
		privateDNS, ok := dnsProvider.(*DNSProviderPrivate)
		if !ok {
			return nil, errors.New("azuredns: unable to create private DNS provider")
		}
		return &DNSProvider{provider: privateDNS}, err
	}

	dnsProvider, err = NewDNSProviderPublic(config, credentials)
	publicDNS, ok := dnsProvider.(*DNSProviderPublic)
	if !ok {
		return nil, errors.New("azuredns: unable to create public DNS provider")
	}
	return &DNSProvider{provider: publicDNS}, err
}

// Timeout returns the timeout and interval to use when checking for DNS propagation.
// Adjusting here to cope with spikes in propagation times.
func (d *DNSProvider) Timeout() (timeout, interval time.Duration) {
	return d.provider.Timeout()
}

// Present creates a TXT record to fulfill the dns-01 challenge.
func (d *DNSProvider) Present(domain, token, keyAuth string) error {
	return d.provider.Present(domain, token, keyAuth)
}

// CleanUp removes the TXT record matching the specified parameters.
func (d *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	return d.provider.CleanUp(domain, token, keyAuth)
}
