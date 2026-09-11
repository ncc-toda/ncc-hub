// Package auth は合言葉(X-Event-Key)・編集キー(X-Edit-Key)の検証と
// 監査ログ(SPEC §7.3, §11)を提供する。
package auth

import (
	"crypto/subtle"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// NewError は SPEC §8.1 のエラー形式 {status, message, data:{code:...}} を返す。
// router.NewApiError は data の値を validation エラーとして解釈して壊すため、
// ApiError を直接組み立てる。
func NewError(status int, code, message string, extra map[string]any) *router.ApiError {
	data := map[string]any{"code": code}
	for k, v := range extra {
		data[k] = v
	}
	return &router.ApiError{Status: status, Message: message, Data: data}
}

// Audit は監査ログを記録する(SPEC §11)。作者名は絶対に渡さないこと。
func Audit(app core.App, action string, args ...any) {
	kv := append([]any{"audit", true, "action", action}, args...)
	app.Logger().Info("audit: "+action, kv...)
}

// RequireEvent は X-Event-Key を検証し、一致した events レコードを返す(SPEC §7.3)。
func RequireEvent(e *core.RequestEvent) (*core.Record, error) {
	return RequireEventFromRequest(e.App, e.Request)
}

// RequireEventFromRequest は http.Request ベースの合言葉検証。
// 標準 Records API のフック(RegisterHooks)からも使う。
func RequireEventFromRequest(app core.App, r *http.Request) (*core.Record, error) {
	key := r.Header.Get("X-Event-Key")
	if key == "" {
		return nil, NewError(http.StatusUnauthorized, "event_key_required", "合言葉を入力してください", nil)
	}
	ev, err := app.FindFirstRecordByFilter("events", "passphrase = {:k}", dbx.Params{"k": key})
	if err != nil {
		Audit(app, "event_key_mismatch", "path", r.URL.Path)
		return nil, NewError(http.StatusUnauthorized, "event_key_invalid", "合言葉が違います", nil)
	}
	return ev, nil
}

// RequireOpen はイベントが受付中かを検証する(SPEC §6.1)。
func RequireOpen(ev *core.Record) error {
	if !ev.GetBool("submissions_open") {
		return NewError(http.StatusLocked, "submissions_closed", "このイベントの受付は終了しました", nil)
	}
	return nil
}

// RequireEditableWork は作品の存在・イベント所属・編集キーを検証する(SPEC §7.3)。
func RequireEditableWork(e *core.RequestEvent, ev *core.Record, workID string) (work *core.Record, secret *core.Record, err error) {
	notFound := NewError(http.StatusNotFound, "not_found", "作品が見つかりません", nil)

	work, findErr := e.App.FindRecordById("works", workID)
	if findErr != nil || work.GetString("event") != ev.Id {
		return nil, nil, notFound
	}

	secret, findErr = e.App.FindFirstRecordByFilter("work_secrets", "work = {:id}", dbx.Params{"id": work.Id})
	if findErr != nil {
		return nil, nil, notFound
	}

	editKey := e.Request.Header.Get("X-Edit-Key")
	if subtle.ConstantTimeCompare([]byte(secret.GetString("edit_key")), []byte(editKey)) != 1 {
		Audit(e.App, "edit_key_mismatch", "work", work.Id, "event", ev.Id)
		return nil, nil, NewError(http.StatusForbidden, "edit_key_invalid", "編集キーが違います", nil)
	}
	return work, secret, nil
}

// RegisterHooks は標準 Records API 向けのフックを登録する。
//   - events / works の list・view で合言葉が無い・違う場合に 401 を返す
//     (APIルールだけだと list は 200 + 空配列になるため。SPEC §15 の受け入れ条件)
//   - イベント設定変更・作品削除の監査ログ(SPEC §11)
func RegisterHooks(app core.App) {
	app.OnRecordsListRequest("events", "works").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if _, err := RequireEventFromRequest(e.App, e.Request); err != nil {
			return err
		}
		return e.Next()
	})

	app.OnRecordViewRequest("events", "works").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if _, err := RequireEventFromRequest(e.App, e.Request); err != nil {
			return err
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("events").BindFunc(func(e *core.RecordEvent) error {
		Audit(e.App, "event_updated", "event", e.Record.Id,
			"submissions_open", e.Record.GetBool("submissions_open"))
		return e.Next()
	})

	app.OnRecordAfterDeleteSuccess("works").BindFunc(func(e *core.RecordEvent) error {
		Audit(e.App, "work_deleted", "work", e.Record.Id, "event", e.Record.GetString("event"))
		return e.Next()
	})
}
