// Package platformconfig is the PlatformConfiguration slice: the single
// per-deployment platform config doc (branding, date format, regions, project
// quota, login config) that the UI bootstraps from and that gates project quota.
// Admin CRUD (master-realm auth) is deferred.
package platformconfig

import "time"

// Branding is the platform branding config. The nested links/theme/scripts objects
// are kept as opaque maps for now (the seed leaves them unset → omitted).
type Branding struct {
	Name       string         `json:"name,omitempty"`
	FaviconURL string         `json:"faviconUrl,omitempty"`
	Logo       string         `json:"logo,omitempty"`
	DarkLogo   string         `json:"darkLogo,omitempty"`
	EmailLogo  string         `json:"emailLogo,omitempty"`
	Code       string         `json:"code,omitempty"`
	Color      string         `json:"color,omitempty"`
	Links      map[string]any `json:"links,omitempty"`
	Theme      map[string]any `json:"theme,omitempty"`
	Scripts    map[string]any `json:"scripts,omitempty"`
}

// DateFormat is the platform date-format config.
type DateFormat struct {
	DateFormat string `json:"dateFormat,omitempty"`
}

// RegionDisplayConfig is one region's display entry.
type RegionDisplayConfig struct {
	ServiceID string `json:"serviceId"`
	Region    string `json:"region"`
	Order     int    `json:"order"`
}

// ProjectProvisioningQuota is the provisioning quota (server-side only; not in the
// client DTO). Read by the project quota check.
type ProjectProvisioningQuota struct {
	Enabled bool `json:"enabled"`
	Limit   int  `json:"limit"`
}

// PlatformConfiguration is the document in the "platformConfiguration" collection.
type PlatformConfiguration struct {
	ID                       string                    `json:"id,omitempty"`
	Name                     string                    `json:"name,omitempty"`
	Language                 string                    `json:"language,omitempty"`
	MailGatewayID            string                    `json:"mailGatewayId,omitempty"`
	Branding                 *Branding                 `json:"branding,omitempty"`
	ContactIntegrationID     string                    `json:"contactIntegrationId,omitempty"`
	SegmentIntegrationID     string                    `json:"segmentIntegrationId,omitempty"`
	DefaultConfiguration     bool                      `json:"defaultConfiguration"`
	Regions                  []RegionDisplayConfig     `json:"regions,omitempty"`
	DateConfiguration        *DateFormat               `json:"dateConfiguration,omitempty"`
	ProjectProvisioningQuota *ProjectProvisioningQuota `json:"projectProvisioningQuota,omitempty"`
	LoginConfiguration       map[string]any            `json:"loginConfiguration,omitempty"`
	CreatedAt                *time.Time                `json:"createdAt,omitempty"`
	UpdatedAt                *time.Time                `json:"updatedAt,omitempty"`
}
