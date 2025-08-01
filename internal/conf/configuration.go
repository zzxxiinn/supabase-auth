package conf

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/gobwas/glob"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"gopkg.in/gomail.v2"
)

const defaultMinPasswordLength int = 6
const defaultChallengeExpiryDuration float64 = 300
const defaultFactorExpiryDuration time.Duration = 300 * time.Second
const defaultFlowStateExpiryDuration time.Duration = 300 * time.Second

// See: https://www.postgresql.org/docs/7.0/syntax525.htm
var postgresNamesRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

// See: https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md
// We use 4 * Math.ceil(n/3) to obtain unpadded length in base 64
// So this 4 * Math.ceil(24/3) = 32 and 4 * Math.ceil(64/3) = 88 for symmetric secrets
// Since Ed25519 key is 32 bytes so we have 4 * Math.ceil(32/3) = 44
var symmetricSecretFormat = regexp.MustCompile(`^v1,whsec_[A-Za-z0-9+/=]{32,88}`)
var asymmetricSecretFormat = regexp.MustCompile(`^v1a,whpk_[A-Za-z0-9+/=]{44,}:whsk_[A-Za-z0-9+/=]{44,}$`)

// Time is used to represent timestamps in the configuration, as envconfig has
// trouble parsing empty strings, due to time.Time.UnmarshalText().
type Time struct {
	time.Time
}

func (t *Time) UnmarshalText(text []byte) error {
	trimed := bytes.TrimSpace(text)

	if len(trimed) < 1 {
		t.Time = time.Time{}
	} else {
		if err := t.Time.UnmarshalText(trimed); err != nil {
			return err
		}
	}

	return nil
}

// OAuthProviderConfiguration holds all config related to external account providers.
type OAuthProviderConfiguration struct {
	ClientID    []string `json:"client_id" split_words:"true"`
	Secret      string   `json:"secret"`
	RedirectURI string   `json:"redirect_uri" split_words:"true"`
	URL         string   `json:"url"`
	ApiURL      string   `json:"api_url" split_words:"true"`
	Enabled     bool     `json:"enabled"`
	// SkipNonceCheck bypasses nonce verification during OIDC token validation.
	// Note: Nonce verification helps prevent replay attacks; only disable when necessary.
	SkipNonceCheck bool `json:"skip_nonce_check" split_words:"true"`
}

type AnonymousProviderConfiguration struct {
	Enabled bool `json:"enabled" default:"false"`
}

type EmailProviderConfiguration struct {
	Enabled bool `json:"enabled" default:"true"`

	AuthorizedAddresses []string `json:"authorized_addresses" split_words:"true"`

	MagicLinkEnabled bool `json:"magic_link_enabled" default:"true" split_words:"true"`
}

// DBConfiguration holds all the database related configuration.
type DBConfiguration struct {
	Driver    string `json:"driver" required:"true"`
	URL       string `json:"url" envconfig:"DATABASE_URL" required:"true"`
	Namespace string `json:"namespace" envconfig:"DB_NAMESPACE" default:"auth"`
	// MaxPoolSize defaults to 0 (unlimited).
	MaxPoolSize       int           `json:"max_pool_size" split_words:"true"`
	MaxIdlePoolSize   int           `json:"max_idle_pool_size" split_words:"true"`
	ConnMaxLifetime   time.Duration `json:"conn_max_lifetime,omitempty" split_words:"true"`
	ConnMaxIdleTime   time.Duration `json:"conn_max_idle_time,omitempty" split_words:"true"`
	HealthCheckPeriod time.Duration `json:"health_check_period" split_words:"true"`
	MigrationsPath    string        `json:"migrations_path" split_words:"true" default:"./migrations"`
	CleanupEnabled    bool          `json:"cleanup_enabled" split_words:"true" default:"false"`
}

func (c *DBConfiguration) Validate() error {
	return nil
}

// JWTConfiguration holds all the JWT related configuration.
type JWTConfiguration struct {
	Secret           string         `json:"secret" required:"true"`
	Exp              int            `json:"exp"`
	Aud              string         `json:"aud"`
	AdminGroupName   string         `json:"admin_group_name" split_words:"true"`
	AdminRoles       []string       `json:"admin_roles" split_words:"true"`
	DefaultGroupName string         `json:"default_group_name" split_words:"true"`
	Issuer           string         `json:"issuer"`
	KeyID            string         `json:"key_id" split_words:"true"`
	Keys             JwtKeysDecoder `json:"keys"`
	ValidMethods     []string       `json:"-"`
}

type MFAFactorTypeConfiguration struct {
	EnrollEnabled bool `json:"enroll_enabled" split_words:"true" default:"false"`
	VerifyEnabled bool `json:"verify_enabled" split_words:"true" default:"false"`
}

type TOTPFactorTypeConfiguration struct {
	EnrollEnabled bool `json:"enroll_enabled" split_words:"true" default:"true"`
	VerifyEnabled bool `json:"verify_enabled" split_words:"true" default:"true"`
}

type PhoneFactorTypeConfiguration struct {
	// Default to false in order to ensure Phone MFA is opt-in
	MFAFactorTypeConfiguration
	OtpLength    int                `json:"otp_length" split_words:"true"`
	SMSTemplate  *template.Template `json:"-"`
	MaxFrequency time.Duration      `json:"max_frequency" split_words:"true"`
	Template     string             `json:"template"`
}

// MFAConfiguration holds all the MFA related Configuration
type MFAConfiguration struct {
	ChallengeExpiryDuration     float64                      `json:"challenge_expiry_duration" default:"300" split_words:"true"`
	FactorExpiryDuration        time.Duration                `json:"factor_expiry_duration" default:"300s" split_words:"true"`
	RateLimitChallengeAndVerify float64                      `split_words:"true" default:"15"`
	MaxEnrolledFactors          float64                      `split_words:"true" default:"10"`
	MaxVerifiedFactors          int                          `split_words:"true" default:"10"`
	Phone                       PhoneFactorTypeConfiguration `split_words:"true"`
	TOTP                        TOTPFactorTypeConfiguration  `split_words:"true"`
	WebAuthn                    MFAFactorTypeConfiguration   `split_words:"true"`
}

type APIConfiguration struct {
	Host               string
	Port               string `envconfig:"PORT" default:"8081"`
	Endpoint           string
	RequestIDHeader    string        `envconfig:"REQUEST_ID_HEADER"`
	ExternalURL        string        `json:"external_url" envconfig:"API_EXTERNAL_URL" required:"true"`
	MaxRequestDuration time.Duration `json:"max_request_duration" split_words:"true" default:"10s"`
}

func (a *APIConfiguration) Validate() error {
	_, err := url.ParseRequestURI(a.ExternalURL)
	if err != nil {
		return err
	}

	return nil
}

type SessionsConfiguration struct {
	Timebox           *time.Duration `json:"timebox,omitempty"`
	InactivityTimeout *time.Duration `json:"inactivity_timeout,omitempty" split_words:"true"`
	AllowLowAAL       *time.Duration `json:"allow_low_aal,omitempty" split_words:"true"`

	SinglePerUser bool     `json:"single_per_user" split_words:"true"`
	Tags          []string `json:"tags,omitempty"`
}

func (c *SessionsConfiguration) Validate() error {
	if c.Timebox != nil && *c.Timebox <= time.Duration(0) {
		return fmt.Errorf("conf: session timebox duration must be positive when set, was %v", (*c.Timebox).String())
	}

	if c.InactivityTimeout != nil && *c.InactivityTimeout <= time.Duration(0) {
		return fmt.Errorf("conf: session inactivity timeout duration must be positive when set, was %v", (*c.InactivityTimeout).String())
	}

	if c.AllowLowAAL != nil && *c.AllowLowAAL <= time.Duration(0) {
		return fmt.Errorf("conf: session allow low AAL duration must be positive when set, was %v", (*c.AllowLowAAL).String())
	}

	return nil
}

type PasswordRequiredCharacters []string

func (v *PasswordRequiredCharacters) Decode(value string) error {
	parts := strings.Split(value, ":")

	for i := 0; i < len(parts)-1; i += 1 {
		part := parts[i]

		if part == "" {
			continue
		}

		// part ended in escape character, so it should be joined with the next one
		if part[len(part)-1] == '\\' {
			parts[i] = part[0:len(part)-1] + ":" + parts[i+1]
			parts[i+1] = ""
			continue
		}
	}

	for _, part := range parts {
		if part != "" {
			*v = append(*v, part)
		}
	}

	return nil
}

// HIBPBloomConfiguration configures a bloom cache for pwned passwords. Use
// this tool to gauge the Items and FalsePositives values:
// https://hur.st/bloomfilter
type HIBPBloomConfiguration struct {
	Enabled        bool    `json:"enabled"`
	Items          uint    `json:"items" default:"100000"`
	FalsePositives float64 `json:"false_positives" split_words:"true" default:"0.0000099"`
}

type HIBPConfiguration struct {
	Enabled    bool `json:"enabled"`
	FailClosed bool `json:"fail_closed" split_words:"true"`

	UserAgent string `json:"user_agent" split_words:"true" default:"https://github.com/supabase/gotrue"`

	Bloom HIBPBloomConfiguration `json:"bloom"`
}

type PasswordConfiguration struct {
	MinLength int `json:"min_length" split_words:"true"`

	RequiredCharacters PasswordRequiredCharacters `json:"required_characters" split_words:"true"`

	HIBP HIBPConfiguration `json:"hibp"`
}

type AuditLogConfiguration struct {
	DisablePostgres bool `split_words:"true" default:"false"`
}

// GlobalConfiguration holds all the configuration that applies to all instances.
type GlobalConfiguration struct {
	API           APIConfiguration
	DB            DBConfiguration
	External      ProviderConfiguration
	Logging       LoggingConfig  `envconfig:"LOG"`
	Profiler      ProfilerConfig `envconfig:"PROFILER"`
	OperatorToken string         `split_words:"true" required:"false"`
	Tracing       TracingConfig
	Metrics       MetricsConfig
	SMTP          SMTPConfiguration
	AuditLog      AuditLogConfiguration `split_words:"true"`

	RateLimitHeader         string  `split_words:"true"`
	RateLimitEmailSent      Rate    `split_words:"true" default:"30"`
	RateLimitSmsSent        Rate    `split_words:"true" default:"30"`
	RateLimitVerify         float64 `split_words:"true" default:"30"`
	RateLimitTokenRefresh   float64 `split_words:"true" default:"150"`
	RateLimitSso            float64 `split_words:"true" default:"30"`
	RateLimitAnonymousUsers float64 `split_words:"true" default:"30"`
	RateLimitOtp            float64 `split_words:"true" default:"30"`
	RateLimitWeb3           float64 `split_words:"true" default:"30"`

	SiteURL         string   `json:"site_url" split_words:"true" required:"true"`
	URIAllowList    []string `json:"uri_allow_list" split_words:"true"`
	URIAllowListMap map[string]glob.Glob
	Password        PasswordConfiguration    `json:"password"`
	JWT             JWTConfiguration         `json:"jwt"`
	Mailer          MailerConfiguration      `json:"mailer"`
	Sms             SmsProviderConfiguration `json:"sms"`
	DisableSignup   bool                     `json:"disable_signup" split_words:"true"`
	Hook            HookConfiguration        `json:"hook" split_words:"true"`
	Security        SecurityConfiguration    `json:"security"`
	Sessions        SessionsConfiguration    `json:"sessions"`
	MFA             MFAConfiguration         `json:"MFA"`
	SAML            SAMLConfiguration        `json:"saml"`
	CORS            CORSConfiguration        `json:"cors"`
}

type CORSConfiguration struct {
	AllowedHeaders []string `json:"allowed_headers" split_words:"true"`
}

func (c *CORSConfiguration) AllAllowedHeaders(defaults []string) []string {
	set := make(map[string]bool)
	for _, header := range defaults {
		set[header] = true
	}

	var result []string
	result = append(result, defaults...)

	for _, header := range c.AllowedHeaders {
		if !set[header] {
			result = append(result, header)
		}

		set[header] = true
	}

	return result
}

// EmailContentConfiguration holds the configuration for emails, both subjects and template URLs.
type EmailContentConfiguration struct {
	Invite           string `json:"invite"`
	Confirmation     string `json:"confirmation"`
	Recovery         string `json:"recovery"`
	EmailChange      string `json:"email_change" split_words:"true"`
	MagicLink        string `json:"magic_link" split_words:"true"`
	Reauthentication string `json:"reauthentication"`
}

type ProviderConfiguration struct {
	AnonymousUsers          AnonymousProviderConfiguration `json:"anonymous_users" split_words:"true"`
	Apple                   OAuthProviderConfiguration     `json:"apple"`
	Azure                   OAuthProviderConfiguration     `json:"azure"`
	Bitbucket               OAuthProviderConfiguration     `json:"bitbucket"`
	Discord                 OAuthProviderConfiguration     `json:"discord"`
	Facebook                OAuthProviderConfiguration     `json:"facebook"`
	Snapchat                OAuthProviderConfiguration     `json:"snapchat"`
	Figma                   OAuthProviderConfiguration     `json:"figma"`
	Fly                     OAuthProviderConfiguration     `json:"fly"`
	Github                  OAuthProviderConfiguration     `json:"github"`
	Gitlab                  OAuthProviderConfiguration     `json:"gitlab"`
	Google                  OAuthProviderConfiguration     `json:"google"`
	Kakao                   OAuthProviderConfiguration     `json:"kakao"`
	Notion                  OAuthProviderConfiguration     `json:"notion"`
	Keycloak                OAuthProviderConfiguration     `json:"keycloak"`
	Linkedin                OAuthProviderConfiguration     `json:"linkedin"`
	LinkedinOIDC            OAuthProviderConfiguration     `json:"linkedin_oidc" envconfig:"LINKEDIN_OIDC"`
	Spotify                 OAuthProviderConfiguration     `json:"spotify"`
	Slack                   OAuthProviderConfiguration     `json:"slack"`
	SlackOIDC               OAuthProviderConfiguration     `json:"slack_oidc" envconfig:"SLACK_OIDC"`
	Twitter                 OAuthProviderConfiguration     `json:"twitter"`
	Twitch                  OAuthProviderConfiguration     `json:"twitch"`
	VercelMarketplace       OAuthProviderConfiguration     `json:"vercel_marketplace" split_words:"true"`
	WorkOS                  OAuthProviderConfiguration     `json:"workos"`
	Email                   EmailProviderConfiguration     `json:"email"`
	Phone                   PhoneProviderConfiguration     `json:"phone"`
	Zoom                    OAuthProviderConfiguration     `json:"zoom"`
	IosBundleId             string                         `json:"ios_bundle_id" split_words:"true"`
	RedirectURL             string                         `json:"redirect_url"`
	AllowedIdTokenIssuers   []string                       `json:"allowed_id_token_issuers" split_words:"true"`
	FlowStateExpiryDuration time.Duration                  `json:"flow_state_expiry_duration" split_words:"true"`

	Web3Solana   SolanaConfiguration   `json:"web3_solana" split_words:"true"`
	Web3Ethereum EthereumConfiguration `json:"web3_ethereum" split_words:"true"`
}

type SolanaConfiguration struct {
	Enabled                 bool          `json:"enabled,omitempty" split_words:"true"`
	MaximumValidityDuration time.Duration `json:"maximum_validity_duration,omitempty" default:"10m" split_words:"true"`
}

type EthereumConfiguration struct {
	Enabled                 bool          `json:"enabled,omitempty" split_words:"true"`
	MaximumValidityDuration time.Duration `json:"maximum_validity_duration,omitempty" default:"10m" split_words:"true"`
}

type SMTPConfiguration struct {
	MaxFrequency   time.Duration `json:"max_frequency" split_words:"true"`
	Host           string        `json:"host"`
	Port           int           `json:"port,omitempty" default:"587"`
	User           string        `json:"user"`
	Pass           string        `json:"pass,omitempty"`
	AdminEmail     string        `json:"admin_email" split_words:"true"`
	SenderName     string        `json:"sender_name" split_words:"true"`
	Headers        string        `json:"headers"`
	LoggingEnabled bool          `json:"logging_enabled" split_words:"true" default:"false"`

	fromAddress       string              `json:"-"`
	normalizedHeaders map[string][]string `json:"-"`
}

func (c *SMTPConfiguration) Validate() error {
	headers := make(map[string][]string)

	if c.Headers != "" {
		err := json.Unmarshal([]byte(c.Headers), &headers)
		if err != nil {
			return fmt.Errorf("conf: SMTP headers not a map[string][]string format: %w", err)
		}
	}

	if len(headers) > 0 {
		c.normalizedHeaders = headers
	}

	mail := gomail.NewMessage()

	c.fromAddress = mail.FormatAddress(c.AdminEmail, c.SenderName)

	return nil
}

func (c *SMTPConfiguration) FromAddress() string {
	return c.fromAddress
}

func (c *SMTPConfiguration) NormalizedHeaders() map[string][]string {
	return c.normalizedHeaders
}

type MailerConfiguration struct {
	Autoconfirm                 bool `json:"autoconfirm"`
	AllowUnverifiedEmailSignIns bool `json:"allow_unverified_email_sign_ins" split_words:"true" default:"false"`

	Subjects  EmailContentConfiguration `json:"subjects"`
	Templates EmailContentConfiguration `json:"templates"`
	URLPaths  EmailContentConfiguration `json:"url_paths"`

	SecureEmailChangeEnabled bool `json:"secure_email_change_enabled" split_words:"true" default:"true"`

	OtpExp    uint `json:"otp_exp" split_words:"true"`
	OtpLength int  `json:"otp_length" split_words:"true"`

	ExternalHosts []string `json:"external_hosts" split_words:"true"`

	// EXPERIMENTAL: May be removed in a future release.
	EmailValidationExtended       bool   `json:"email_validation_extended" split_words:"true" default:"false"`
	EmailValidationServiceURL     string `json:"email_validation_service_url" split_words:"true"`
	EmailValidationServiceHeaders string `json:"email_validation_service_headers" split_words:"true"`
	EmailValidationBlockedMX      string `json:"email_validation_blocked_mx" split_words:"true"`

	serviceHeaders   map[string][]string `json:"-"`
	blockedMXRecords map[string]bool     `json:"-"`
}

func (c *MailerConfiguration) Validate() error {
	headers := make(map[string][]string)

	if c.EmailValidationServiceHeaders != "" {
		err := json.Unmarshal([]byte(c.EmailValidationServiceHeaders), &headers)
		if err != nil {
			return fmt.Errorf("conf: mailer validation headers not a map[string][]string format: %w", err)
		}
	}

	if len(headers) > 0 {
		c.serviceHeaders = headers
	}

	// EmailValidationBlockedMX is a JSON array in the config string for brevity.
	var blockedMXRecords map[string]bool
	if c.EmailValidationBlockedMX != "" {
		var blockedMXArray []string
		err := json.Unmarshal([]byte(c.EmailValidationBlockedMX), &blockedMXArray)
		if err != nil {
			return fmt.Errorf("conf: email_validation_blocked_mx is not a valid JSON array: %w", err)
		}
		blockedMXRecords = make(map[string]bool, len(blockedMXArray)*2)
		for _, record := range blockedMXArray {
			blockedMXRecords[record] = true
			blockedMXRecords[record+"."] = true
		}
	}

	c.blockedMXRecords = blockedMXRecords

	return nil
}

func (c *MailerConfiguration) GetEmailValidationServiceHeaders() map[string][]string {
	return c.serviceHeaders
}

func (c *MailerConfiguration) GetEmailValidationBlockedMXRecords() map[string]bool {
	return c.blockedMXRecords
}

type PhoneProviderConfiguration struct {
	Enabled bool `json:"enabled" default:"false"`
}

type SmsProviderConfiguration struct {
	Autoconfirm       bool               `json:"autoconfirm"`
	MaxFrequency      time.Duration      `json:"max_frequency" split_words:"true"`
	OtpExp            uint               `json:"otp_exp" split_words:"true"`
	OtpLength         int                `json:"otp_length" split_words:"true"`
	Provider          string             `json:"provider"`
	Template          string             `json:"template"`
	TestOTP           map[string]string  `json:"test_otp" split_words:"true"`
	TestOTPValidUntil Time               `json:"test_otp_valid_until" split_words:"true"`
	SMSTemplate       *template.Template `json:"-"`

	Twilio       TwilioProviderConfiguration       `json:"twilio"`
	TwilioVerify TwilioVerifyProviderConfiguration `json:"twilio_verify" split_words:"true"`
	Messagebird  MessagebirdProviderConfiguration  `json:"messagebird"`
	Textlocal    TextlocalProviderConfiguration    `json:"textlocal"`
	Vonage       VonageProviderConfiguration       `json:"vonage"`
	Aliyun       AliyunProviderConfiguration       `json:"aliyun"`
}

func (c *SmsProviderConfiguration) GetTestOTP(phone string, now time.Time) (string, bool) {
	if c.TestOTP != nil && (c.TestOTPValidUntil.Time.IsZero() || now.Before(c.TestOTPValidUntil.Time)) {
		testOTP, ok := c.TestOTP[phone]
		return testOTP, ok
	}

	return "", false
}

type TwilioProviderConfiguration struct {
	AccountSid        string `json:"account_sid" split_words:"true"`
	AuthToken         string `json:"auth_token" split_words:"true"`
	MessageServiceSid string `json:"message_service_sid" split_words:"true"`
	ContentSid        string `json:"content_sid" split_words:"true"`
}

type TwilioVerifyProviderConfiguration struct {
	AccountSid        string `json:"account_sid" split_words:"true"`
	AuthToken         string `json:"auth_token" split_words:"true"`
	MessageServiceSid string `json:"message_service_sid" split_words:"true"`
}

type MessagebirdProviderConfiguration struct {
	AccessKey  string `json:"access_key" split_words:"true"`
	Originator string `json:"originator" split_words:"true"`
}

type TextlocalProviderConfiguration struct {
	ApiKey string `json:"api_key" split_words:"true"`
	Sender string `json:"sender" split_words:"true"`
}

type VonageProviderConfiguration struct {
	ApiKey    string `json:"api_key" split_words:"true"`
	ApiSecret string `json:"api_secret" split_words:"true"`
	From      string `json:"from" split_words:"true"`
}

type CaptchaConfiguration struct {
	Enabled  bool   `json:"enabled" default:"false"`
	Provider string `json:"provider" default:"hcaptcha"`
	Secret   string `json:"provider_secret"`
}

func (c *CaptchaConfiguration) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Provider != "hcaptcha" && c.Provider != "turnstile" {
		return fmt.Errorf("unsupported captcha provider: %s", c.Provider)
	}

	c.Secret = strings.TrimSpace(c.Secret)

	if c.Secret == "" {
		return errors.New("captcha provider secret is empty")
	}

	return nil
}

// DatabaseEncryptionConfiguration configures Auth to encrypt certain columns.
// Once Encrypt is set to true, data will start getting encrypted with the
// provided encryption key. Setting it to false just stops encryption from
// going on further, but DecryptionKeys would have to contain the same key so
// the encrypted data remains accessible.
type DatabaseEncryptionConfiguration struct {
	Encrypt bool `json:"encrypt"`

	EncryptionKeyID string `json:"encryption_key_id" split_words:"true"`
	EncryptionKey   string `json:"-" split_words:"true"`

	DecryptionKeys map[string]string `json:"-" split_words:"true"`
}

func (c *DatabaseEncryptionConfiguration) Validate() error {
	if c.Encrypt {
		if c.EncryptionKeyID == "" {
			return errors.New("conf: encryption key ID must be specified")
		}

		decodedKey, err := base64.RawURLEncoding.DecodeString(c.EncryptionKey)
		if err != nil {
			return err
		}

		if len(decodedKey) != 256/8 {
			return errors.New("conf: encryption key is not 256 bits")
		}

		if c.DecryptionKeys == nil || c.DecryptionKeys[c.EncryptionKeyID] == "" {
			return errors.New("conf: encryption key must also be present in decryption keys")
		}
	}

	for id, key := range c.DecryptionKeys {
		decodedKey, err := base64.RawURLEncoding.DecodeString(key)
		if err != nil {
			return err
		}

		if len(decodedKey) != 256/8 {
			return fmt.Errorf("conf: decryption key with ID %q must be 256 bits", id)
		}
	}

	return nil
}

type SecurityConfiguration struct {
	Captcha                               CaptchaConfiguration `json:"captcha"`
	RefreshTokenRotationEnabled           bool                 `json:"refresh_token_rotation_enabled" split_words:"true" default:"true"`
	RefreshTokenReuseInterval             int                  `json:"refresh_token_reuse_interval" split_words:"true"`
	UpdatePasswordRequireReauthentication bool                 `json:"update_password_require_reauthentication" split_words:"true"`
	ManualLinkingEnabled                  bool                 `json:"manual_linking_enabled" split_words:"true" default:"false"`

	DBEncryption DatabaseEncryptionConfiguration `json:"database_encryption" split_words:"true"`
}

func (c *SecurityConfiguration) Validate() error {
	if err := c.Captcha.Validate(); err != nil {
		return err
	}

	if err := c.DBEncryption.Validate(); err != nil {
		return err
	}

	return nil
}

func loadEnvironment(filename string) error {
	var err error
	if filename != "" {
		err = godotenv.Overload(filename)
	} else {
		err = godotenv.Load()
		// handle if .env file does not exist, this is OK
		if os.IsNotExist(err) {
			return nil
		}
	}
	return err
}

// Moving away from the existing HookConfig so we can get a fresh start.
type HookConfiguration struct {
	MFAVerificationAttempt      ExtensibilityPointConfiguration `json:"mfa_verification_attempt" split_words:"true"`
	PasswordVerificationAttempt ExtensibilityPointConfiguration `json:"password_verification_attempt" split_words:"true"`
	CustomAccessToken           ExtensibilityPointConfiguration `json:"custom_access_token" split_words:"true"`
	SendEmail                   ExtensibilityPointConfiguration `json:"send_email" split_words:"true"`
	SendSMS                     ExtensibilityPointConfiguration `json:"send_sms" split_words:"true"`

	BeforeUserCreated ExtensibilityPointConfiguration `json:"before_user_created" split_words:"true"`
	AfterUserCreated  ExtensibilityPointConfiguration `json:"after_user_created" split_words:"true"`
}

type HTTPHookSecrets []string

func (h *HTTPHookSecrets) Decode(value string) error {
	parts := strings.Split(value, "|")
	for _, part := range parts {
		if part != "" {
			*h = append(*h, part)
		}
	}

	return nil
}

type ExtensibilityPointConfiguration struct {
	URI     string `json:"uri"`
	Enabled bool   `json:"enabled"`
	// For internal use together with Postgres Hook. Not publicly exposed.
	HookName string `json:"-"`
	// We use | as a separator for keys and : as a separator for keys within a keypair. For instance: v1,whsec_test|v1a,whpk_myother:v1a,whsk_testkey|v1,whsec_secret3
	HTTPHookSecrets HTTPHookSecrets `json:"secrets" envconfig:"secrets"`
}

func (h *HookConfiguration) Validate() error {
	points := []ExtensibilityPointConfiguration{
		h.MFAVerificationAttempt,
		h.PasswordVerificationAttempt,
		h.CustomAccessToken,
		h.SendSMS,
		h.SendEmail,
		h.BeforeUserCreated,
		h.AfterUserCreated,
	}
	for _, point := range points {
		if err := point.ValidateExtensibilityPoint(); err != nil {
			return err
		}
	}
	return nil
}

func (e *ExtensibilityPointConfiguration) ValidateExtensibilityPoint() error {
	if e.URI == "" {
		return nil
	}
	u, err := url.Parse(e.URI)
	if err != nil {
		return err
	}
	switch strings.ToLower(u.Scheme) {
	case "pg-functions":
		return validatePostgresPath(u)
	case "http":
		hostname := u.Hostname()
		if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || hostname == "host.docker.internal" {
			return validateHTTPHookSecrets(e.HTTPHookSecrets)
		}
		return fmt.Errorf("only localhost, 127.0.0.1, and ::1 are supported with http")
	case "https":
		return validateHTTPHookSecrets(e.HTTPHookSecrets)
	default:
		return fmt.Errorf("only postgres hooks and HTTPS functions are supported at the moment")
	}
}

func validatePostgresPath(u *url.URL) error {
	pathParts := strings.Split(u.Path, "/")
	if len(pathParts) < 3 {
		return fmt.Errorf("URI path does not contain enough parts")
	}

	schema := pathParts[1]
	table := pathParts[2]
	// Validate schema and table names
	if !postgresNamesRegexp.MatchString(schema) {
		return fmt.Errorf("invalid schema name: %s", schema)
	}
	if !postgresNamesRegexp.MatchString(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	return nil
}

func isValidSecretFormat(secret string) bool {
	return symmetricSecretFormat.MatchString(secret) || asymmetricSecretFormat.MatchString(secret)
}

func validateHTTPHookSecrets(secrets []string) error {
	for _, secret := range secrets {
		if !isValidSecretFormat(secret) {
			return fmt.Errorf("invalid secret format")
		}
	}
	return nil
}

func (e *ExtensibilityPointConfiguration) PopulateExtensibilityPoint() error {
	u, err := url.Parse(e.URI)
	if err != nil {
		return err
	}
	if u.Scheme == "pg-functions" {
		pathParts := strings.Split(u.Path, "/")
		e.HookName = fmt.Sprintf("%q.%q", pathParts[1], pathParts[2])
	}
	return nil
}

// LoadFile calls godotenv.Load() when the given filename is empty ignoring any
// errors loading, otherwise it calls godotenv.Overload(filename).
//
// godotenv.Load: preserves env, ".env" path is optional
// godotenv.Overload: overrides env, "filename" path must exist
func LoadFile(filename string) error {
	var err error
	if filename != "" {
		err = godotenv.Overload(filename)
	} else {
		err = godotenv.Load()
		// handle if .env file does not exist, this is OK
		if os.IsNotExist(err) {
			return nil
		}
	}
	return err
}

// LoadDirectory does nothing when configDir is empty, otherwise it will attempt
// to load a list of configuration files located in configDir by using ReadDir
// to obtain a sorted list of files containing a .env suffix.
//
// When the list is empty it will do nothing, otherwise it passes the file list
// to godotenv.Overload to pull them into the current environment.
func LoadDirectory(configDir string) error {
	if configDir == "" {
		return nil
	}

	// Returns entries sorted by filename
	ents, err := os.ReadDir(configDir)
	if err != nil {
		// We mimic the behavior of LoadGlobal here, if an explicit path is
		// provided we return an error.
		return err
	}

	var paths []string
	for _, ent := range ents {
		if ent.IsDir() {
			continue // ignore directories
		}

		// We only read files ending in .env
		name := ent.Name()
		if !strings.HasSuffix(name, ".env") {
			continue
		}

		// ent.Name() does not include the watch dir.
		paths = append(paths, filepath.Join(configDir, name))
	}

	// If at least one path was found we load the configuration files in the
	// directory. We don't call override without config files because it will
	// override the env vars previously set with a ".env", if one exists.
	return loadDirectoryPaths(paths...)
}

func loadDirectoryPaths(p ...string) error {
	// If at least one path was found we load the configuration files in the
	// directory. We don't call override without config files because it will
	// override the env vars previously set with a ".env", if one exists.
	if len(p) > 0 {
		if err := godotenv.Overload(p...); err != nil {
			return err
		}
	}
	return nil
}

// LoadGlobalFromEnv will return a new *GlobalConfiguration value from the
// currently configured environment.
func LoadGlobalFromEnv() (*GlobalConfiguration, error) {
	config := new(GlobalConfiguration)
	if err := loadGlobal(config); err != nil {
		return nil, err
	}
	return config, nil
}

func LoadGlobal(filename string) (*GlobalConfiguration, error) {
	if err := loadEnvironment(filename); err != nil {
		return nil, err
	}

	config := new(GlobalConfiguration)
	if err := loadGlobal(config); err != nil {
		return nil, err
	}
	return config, nil
}

func loadGlobal(config *GlobalConfiguration) error {
	// although the package is called "auth" it used to be called "gotrue"
	// so environment configs will remain to be called "GOTRUE"
	if err := envconfig.Process("gotrue", config); err != nil {
		return err
	}

	if err := config.ApplyDefaults(); err != nil {
		return err
	}

	if err := config.Validate(); err != nil {
		return err
	}
	return populateGlobal(config)
}

func populateGlobal(config *GlobalConfiguration) error {
	if config.Hook.PasswordVerificationAttempt.Enabled {
		if err := config.Hook.PasswordVerificationAttempt.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}

	if config.Hook.SendSMS.Enabled {
		if err := config.Hook.SendSMS.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}
	if config.Hook.SendEmail.Enabled {
		if err := config.Hook.SendEmail.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}

	if config.Hook.MFAVerificationAttempt.Enabled {
		if err := config.Hook.MFAVerificationAttempt.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}

	if config.Hook.CustomAccessToken.Enabled {
		if err := config.Hook.CustomAccessToken.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}

	if config.Hook.BeforeUserCreated.Enabled {
		if err := config.Hook.BeforeUserCreated.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}

	if config.Hook.AfterUserCreated.Enabled {
		if err := config.Hook.AfterUserCreated.PopulateExtensibilityPoint(); err != nil {
			return err
		}
	}

	if config.SAML.Enabled {
		if err := config.SAML.PopulateFields(config.API.ExternalURL); err != nil {
			return err
		}
	} else {
		config.SAML.PrivateKey = ""
	}

	if config.Sms.Provider != "" {
		SMSTemplate := config.Sms.Template
		if SMSTemplate == "" {
			SMSTemplate = "Your code is {{ .Code }}"
		}
		template, err := template.New("").Parse(SMSTemplate)
		if err != nil {
			return err
		}
		config.Sms.SMSTemplate = template
	}

	if config.MFA.Phone.EnrollEnabled || config.MFA.Phone.VerifyEnabled {
		smsTemplate := config.MFA.Phone.Template
		if smsTemplate == "" {
			smsTemplate = "Your code is {{ .Code }}"
		}
		template, err := template.New("").Parse(smsTemplate)
		if err != nil {
			return err
		}
		config.MFA.Phone.SMSTemplate = template
	}

	return nil
}

// ApplyDefaults sets defaults for a GlobalConfiguration
func (config *GlobalConfiguration) ApplyDefaults() error {
	if config.JWT.AdminGroupName == "" {
		config.JWT.AdminGroupName = "admin"
	}

	if len(config.JWT.AdminRoles) == 0 {
		config.JWT.AdminRoles = []string{"service_role", "supabase_admin"}
	}

	if config.JWT.Exp == 0 {
		config.JWT.Exp = 3600
	}

	if len(config.JWT.Keys) == 0 {
		// transform the secret into a JWK for consistency
		if err := config.applyDefaultsJWT([]byte(config.JWT.Secret)); err != nil {
			return err
		}
	}

	if config.JWT.ValidMethods == nil {
		config.JWT.ValidMethods = []string{}
		for _, key := range config.JWT.Keys {
			alg := GetSigningAlg(key.PublicKey)
			config.JWT.ValidMethods = append(config.JWT.ValidMethods, alg.Alg())
		}

	}

	if config.Mailer.Autoconfirm && config.Mailer.AllowUnverifiedEmailSignIns {
		return errors.New("cannot enable both GOTRUE_MAILER_AUTOCONFIRM and GOTRUE_MAILER_ALLOW_UNVERIFIED_EMAIL_SIGN_INS")
	}

	if config.Mailer.URLPaths.Invite == "" {
		config.Mailer.URLPaths.Invite = "/verify"
	}

	if config.Mailer.URLPaths.Confirmation == "" {
		config.Mailer.URLPaths.Confirmation = "/verify"
	}

	if config.Mailer.URLPaths.Recovery == "" {
		config.Mailer.URLPaths.Recovery = "/verify"
	}

	if config.Mailer.URLPaths.EmailChange == "" {
		config.Mailer.URLPaths.EmailChange = "/verify"
	}

	if config.Mailer.OtpExp == 0 {
		config.Mailer.OtpExp = 86400 // 1 day
	}

	if config.Mailer.OtpLength == 0 || config.Mailer.OtpLength < 6 || config.Mailer.OtpLength > 10 {
		// 6-digit otp by default
		config.Mailer.OtpLength = 6
	}

	if config.SMTP.MaxFrequency == 0 {
		config.SMTP.MaxFrequency = 1 * time.Minute
	}

	if config.Sms.MaxFrequency == 0 {
		config.Sms.MaxFrequency = 1 * time.Minute
	}

	if config.Sms.OtpExp == 0 {
		config.Sms.OtpExp = 60
	}

	if config.Sms.OtpLength == 0 || config.Sms.OtpLength < 6 || config.Sms.OtpLength > 10 {
		// 6-digit otp by default
		config.Sms.OtpLength = 6
	}

	if config.Sms.TestOTP != nil {
		formatTestOtps := make(map[string]string)
		for phone, otp := range config.Sms.TestOTP {
			phone = strings.ReplaceAll(strings.TrimPrefix(phone, "+"), " ", "")
			formatTestOtps[phone] = otp
		}
		config.Sms.TestOTP = formatTestOtps
	}

	if len(config.Sms.Template) == 0 {
		config.Sms.Template = ""
	}

	if config.URIAllowList == nil {
		config.URIAllowList = []string{}
	}

	if config.URIAllowList != nil {
		config.URIAllowListMap = make(map[string]glob.Glob)
		for _, uri := range config.URIAllowList {
			g := glob.MustCompile(uri, '.', '/')
			config.URIAllowListMap[uri] = g
		}
	}

	if config.Password.MinLength < defaultMinPasswordLength {
		config.Password.MinLength = defaultMinPasswordLength
	}

	if config.MFA.ChallengeExpiryDuration < defaultChallengeExpiryDuration {
		config.MFA.ChallengeExpiryDuration = defaultChallengeExpiryDuration
	}

	if config.MFA.FactorExpiryDuration < defaultFactorExpiryDuration {
		config.MFA.FactorExpiryDuration = defaultFactorExpiryDuration
	}

	if config.MFA.Phone.MaxFrequency == 0 {
		config.MFA.Phone.MaxFrequency = 1 * time.Minute
	}

	if config.MFA.Phone.OtpLength < 6 || config.MFA.Phone.OtpLength > 10 {
		// 6-digit otp by default
		config.MFA.Phone.OtpLength = 6
	}

	if config.External.FlowStateExpiryDuration < defaultFlowStateExpiryDuration {
		config.External.FlowStateExpiryDuration = defaultFlowStateExpiryDuration
	}

	if len(config.External.AllowedIdTokenIssuers) == 0 {
		config.External.AllowedIdTokenIssuers = append(config.External.AllowedIdTokenIssuers, "https://appleid.apple.com", "https://accounts.google.com")
	}

	return nil
}
func (config *GlobalConfiguration) applyDefaultsJWT(secret []byte) error {
	// transform the secret into a JWK for consistency
	privKey, err := jwk.FromRaw(secret)
	if err != nil {
		return err
	}
	return config.applyDefaultsJWTPrivateKey(privKey)
}

func (config *GlobalConfiguration) applyDefaultsJWTPrivateKey(privKey jwk.Key) error {
	if config.JWT.KeyID != "" {
		if err := privKey.Set(jwk.KeyIDKey, config.JWT.KeyID); err != nil {
			return err
		}
	}
	if privKey.Algorithm().String() == "" {
		if err := privKey.Set(jwk.AlgorithmKey, jwt.SigningMethodHS256.Name); err != nil {
			return err
		}
	}
	if err := privKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return err
	}
	if len(privKey.KeyOps()) == 0 {
		if err := privKey.Set(jwk.KeyOpsKey, jwk.KeyOperationList{jwk.KeyOpSign, jwk.KeyOpVerify}); err != nil {
			return err
		}
	}
	pubKey, err := privKey.PublicKey()
	if err != nil {
		return err
	}
	config.JWT.Keys = make(JwtKeysDecoder)
	config.JWT.Keys[config.JWT.KeyID] = JwkInfo{
		PublicKey:  pubKey,
		PrivateKey: privKey,
	}
	return nil
}

// Validate validates all of configuration.
func (c *GlobalConfiguration) Validate() error {
	validatables := []interface {
		Validate() error
	}{
		&c.API,
		&c.DB,
		&c.Tracing,
		&c.Metrics,
		&c.SMTP,
		&c.Mailer,
		&c.SAML,
		&c.Security,
		&c.Sessions,
		&c.Hook,
		&c.JWT.Keys,
	}

	for _, validatable := range validatables {
		if err := validatable.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func (o *OAuthProviderConfiguration) ValidateOAuth() error {
	if !o.Enabled {
		return errors.New("provider is not enabled")
	}
	if len(o.ClientID) == 0 {
		return errors.New("missing OAuth client ID")
	}
	if o.Secret == "" {
		return errors.New("missing OAuth secret")
	}
	if o.RedirectURI == "" {
		return errors.New("missing redirect URI")
	}
	return nil
}

func (t *TwilioProviderConfiguration) Validate() error {
	if t.AccountSid == "" {
		return errors.New("missing Twilio account SID")
	}
	if t.AuthToken == "" {
		return errors.New("missing Twilio auth token")
	}
	if t.MessageServiceSid == "" {
		return errors.New("missing Twilio message service SID or Twilio phone number")
	}
	return nil
}

func (t *TwilioVerifyProviderConfiguration) Validate() error {
	if t.AccountSid == "" {
		return errors.New("missing Twilio account SID")
	}
	if t.AuthToken == "" {
		return errors.New("missing Twilio auth token")
	}
	if t.MessageServiceSid == "" {
		return errors.New("missing Twilio message service SID or Twilio phone number")
	}
	return nil
}

func (t *MessagebirdProviderConfiguration) Validate() error {
	if t.AccessKey == "" {
		return errors.New("missing Messagebird access key")
	}
	if t.Originator == "" {
		return errors.New("missing Messagebird originator")
	}
	return nil
}

func (t *TextlocalProviderConfiguration) Validate() error {
	if t.ApiKey == "" {
		return errors.New("missing Textlocal API key")
	}
	if t.Sender == "" {
		return errors.New("missing Textlocal sender")
	}
	return nil
}

func (t *VonageProviderConfiguration) Validate() error {
	if t.ApiKey == "" {
		return errors.New("missing Vonage API key")
	}
	if t.ApiSecret == "" {
		return errors.New("missing Vonage API secret")
	}
	if t.From == "" {
		return errors.New("missing Vonage 'from' parameter")
	}
	return nil
}

func (t *SmsProviderConfiguration) IsTwilioVerifyProvider() bool {
	return t.Provider == "twilio_verify"
}

type AliyunProviderConfiguration struct {
	AccessKeyId     string `json:"access_key_id" split_words:"true"`
	AccessKeySecret string `json:"access_key_secret" split_words:"true"`
	Endpoint        string `json:"endpoint" split_words:"true"`
	SignName        string `json:"sign_name" split_words:"true"`
	SmsUpExtendCode string `json:"sms_up_extend_code" split_words:"true"`
}

func (t *AliyunProviderConfiguration) Validate() error {
	if t.AccessKeyId == "" {
		return errors.New("missing Aliyun Access Key ID")
	}
	if t.AccessKeySecret == "" {
		return errors.New("missing Aliyun Access Key Secret")
	}
	if t.Endpoint == "" {
		t.Endpoint = "https://dysmsapi.aliyuncs.com"
	}
	if t.SignName == "" {
		return errors.New("missing Aliyun Sign Name")
	}

	return nil
}
