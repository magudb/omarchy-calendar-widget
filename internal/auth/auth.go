package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"
)

const (
	// Common tenant allows personal and work accounts
	CommonTenant = "common"
	// Local redirect URI for browser authentication
	RedirectURI = "http://localhost:12345/auth/callback"
)

// Scopes required for calendar access
var Scopes = []string{
	"https://graph.microsoft.com/Calendars.Read",
	"https://graph.microsoft.com/User.Read",
}

type Config struct {
	ClientID    string `json:"client_id"`
	TenantID    string `json:"tenant_id"`
	RedirectURI string `json:"redirect_uri"`
}

func GetConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "calendar-widget", "config.json")
}

func getAuthRecordPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "calendar-widget", "auth_record.json")
}

func loadAuthRecord() (*azidentity.AuthenticationRecord, error) {
	data, err := os.ReadFile(getAuthRecordPath())
	if err != nil {
		return nil, err
	}
	var record azidentity.AuthenticationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func saveAuthRecord(record azidentity.AuthenticationRecord) error {
	configDir := filepath.Dir(getAuthRecordPath())
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(getAuthRecordPath(), data, 0600)
}

func LoadConfig() (*Config, error) {
	configPath := GetConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config found - run 'calendar-widget setup' first")
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

func SaveConfig(config *Config) error {
	configPath := GetConfigPath()
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0600)
}

// tokenCache is a persistent cache that stores tokens in the OS keyring
var tokenCache azidentity.Cache
var cacheInitialized bool

func init() {
	var err error
	tokenCache, err = cache.New(&cache.Options{Name: "calendar-widget"})
	cacheInitialized = err == nil
}

// GetCredential returns a credential that uses the persistent token cache.
func GetCredential(allowInteractive bool) (azcore.TokenCredential, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	opts := &azidentity.InteractiveBrowserCredentialOptions{
		ClientID:    config.ClientID,
		TenantID:    config.TenantID,
		RedirectURL: config.RedirectURI,
	}

	// Enable persistent token cache if available
	if cacheInitialized {
		opts.Cache = tokenCache
	}

	// Load existing auth record for silent authentication
	if record, err := loadAuthRecord(); err == nil {
		opts.AuthenticationRecord = *record
	}

	// DisableAutomaticAuthentication prevents browser popup
	if !allowInteractive {
		opts.DisableAutomaticAuthentication = true
	}

	credential, err := azidentity.NewInteractiveBrowserCredential(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	return credential, nil
}

// GetAccessToken returns an access token, using cached tokens when available.
// Set allowInteractive=true to allow browser login if no cached token exists.
func GetAccessToken(ctx context.Context, allowInteractive bool) (azcore.AccessToken, error) {
	config, err := LoadConfig()
	if err != nil {
		return azcore.AccessToken{}, err
	}

	opts := &azidentity.InteractiveBrowserCredentialOptions{
		ClientID:    config.ClientID,
		TenantID:    config.TenantID,
		RedirectURL: config.RedirectURI,
	}

	if cacheInitialized {
		opts.Cache = tokenCache
	}

	if record, err := loadAuthRecord(); err == nil {
		opts.AuthenticationRecord = *record
	}

	if !allowInteractive {
		opts.DisableAutomaticAuthentication = true
	}

	credential, err := azidentity.NewInteractiveBrowserCredential(opts)
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("failed to create credential: %w", err)
	}

	// If interactive, authenticate and save the record for future silent auth
	if allowInteractive {
		record, err := credential.Authenticate(ctx, &policy.TokenRequestOptions{
			Scopes: Scopes,
		})
		if err != nil {
			return azcore.AccessToken{}, fmt.Errorf("failed to authenticate: %w", err)
		}
		_ = saveAuthRecord(record)
	}

	token, err := credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: Scopes,
	})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("failed to get access token: %w", err)
	}

	return token, nil
}
