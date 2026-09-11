// Package likes はいいねのトグルと状態復元を提供する(SPEC §8.6)。
package likes

import (
	"net/http"
	"regexp"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/ncc-toda/ncc-hub/server/internal/auth"
)

// UUID v4 形式(SPEC §7.1)。
var uuidV4Pattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// Register はカスタムルートを登録する。
func Register(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/x/works/{id}/like", toggle)
		se.Router.GET("/api/x/likes", list)
		return se.Next()
	})
}

func requireDeviceID(e *core.RequestEvent) (string, error) {
	deviceID := e.Request.Header.Get("X-Device-Id")
	if !uuidV4Pattern.MatchString(deviceID) {
		return "", auth.NewError(http.StatusBadRequest, "bad_request", "端末IDが不正です", nil)
	}
	return deviceID, nil
}

// POST /api/x/works/{id}/like — トグル(SPEC §8.6)。
func toggle(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	deviceID, err := requireDeviceID(e)
	if err != nil {
		return err
	}

	work, err := e.App.FindRecordById("works", e.Request.PathValue("id"))
	if err != nil || work.GetString("event") != ev.Id {
		return auth.NewError(http.StatusNotFound, "not_found", "作品が見つかりません", nil)
	}

	var liked bool
	var likeCount int64

	txErr := e.App.RunInTransaction(func(tx core.App) error {
		existing, err := tx.FindFirstRecordByFilter("reactions",
			"work = {:w} && device_id = {:d}",
			dbx.Params{"w": work.Id, "d": deviceID})
		if err == nil {
			if err := tx.Delete(existing); err != nil {
				return err
			}
			liked = false
		} else {
			col, err := tx.FindCollectionByNameOrId("reactions")
			if err != nil {
				return err
			}
			reaction := core.NewRecord(col)
			reaction.Set("work", work.Id)
			reaction.Set("device_id", deviceID)
			if err := tx.Save(reaction); err != nil {
				return err
			}
			liked = true
		}

		// 差分更新にせず毎回 COUNT で再集計する(ズレの自己修復。SPEC §8.6)。
		likeCount, err = tx.CountRecords("reactions", dbx.HashExp{"work": work.Id})
		if err != nil {
			return err
		}
		work.Set("like_count", likeCount)
		return tx.Save(work)
	})
	if txErr != nil {
		return txErr
	}

	return e.JSON(http.StatusOK, map[string]any{
		"liked":      liked,
		"like_count": likeCount,
	})
}

// GET /api/x/likes?event=<event_id> — 端末のいいね済み作品ID一覧(SPEC §8.6)。
func list(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	deviceID, err := requireDeviceID(e)
	if err != nil {
		return err
	}
	// クエリの event は合言葉が示すイベントと一致している必要がある。
	if q := e.Request.URL.Query().Get("event"); q != "" && q != ev.Id {
		return auth.NewError(http.StatusNotFound, "not_found", "イベントが見つかりません", nil)
	}

	reactions, err := e.App.FindRecordsByFilter("reactions",
		"device_id = {:d} && work.event = {:e}", "", 0, 0,
		dbx.Params{"d": deviceID, "e": ev.Id})
	if err != nil {
		return err
	}

	workIDs := make([]string, 0, len(reactions))
	for _, r := range reactions {
		workIDs = append(workIDs, r.GetString("work"))
	}
	return e.JSON(http.StatusOK, map[string]any{"work_ids": workIDs})
}
