package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(SettingsUp, settingsDown, "1757570001_settings.go")
}

// SettingsUp はレートリミット(SPEC §7.4)とバックアップ(SPEC §12.5)の設定を投入する。
func SettingsUp(app core.App) error {
	s := app.Settings()

	s.RateLimits.Enabled = true
	s.RateLimits.Rules = []core.RateLimitRule{
		{Label: "POST /api/x/works", MaxRequests: 10, Duration: 60},
		{Label: "PUT /api/x/works/", MaxRequests: 120, Duration: 60},
		{Label: "POST /api/x/works/", MaxRequests: 60, Duration: 60},
		{Label: "/api/collections/", MaxRequests: 300, Duration: 60},
		{Label: "POST /api/collections/_superusers/auth-with-password", MaxRequests: 5, Duration: 60},
	}

	s.Backups.Cron = "0 3 * * *"
	s.Backups.CronMaxKeep = 7

	return app.Save(s)
}

func settingsDown(app core.App) error {
	s := app.Settings()
	s.RateLimits.Enabled = false
	s.RateLimits.Rules = nil
	s.Backups.Cron = ""
	s.Backups.CronMaxKeep = 0
	return app.Save(s)
}
