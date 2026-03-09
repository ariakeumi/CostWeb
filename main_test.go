package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestApp(t *testing.T) *app {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?mode=memory")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := initSchema(context.Background(), db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	if err := seedAssets(context.Background(), db); err != nil {
		t.Fatalf("seed assets: %v", err)
	}

	return &app{db: db}
}

func TestDashboardReturnsAnalytics(t *testing.T) {
	application := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	application.handleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var response dashboardResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Assets) == 0 {
		t.Fatal("expected seeded assets")
	}
	var expectedDaily float64
	for _, asset := range response.Assets {
		expectedDaily += asset.DailyCost
	}
	if round2(expectedDaily) != response.Summary.AverageDaily {
		t.Fatalf("expected total daily cost %.2f, got %.2f", round2(expectedDaily), response.Summary.AverageDaily)
	}
	if len(response.Analytics.Categories) == 0 {
		t.Fatal("expected category analytics")
	}
	if len(response.Analytics.MonthlySpend) == 0 {
		t.Fatal("expected monthly trend data")
	}
}

func TestCreateUpdateAndDeleteAsset(t *testing.T) {
	application := newTestApp(t)

	t.Run("create sold asset persists category and sold fields", func(t *testing.T) {
		body := `{"name":"Kindle Oasis","icon":"📚","category":"阅读","price":1899,"purchaseDate":"2025-10-01","status":"sold","soldPrice":1200,"soldDate":"2026-02-01"}`
		req := httptest.NewRequest(http.MethodPost, "/api/assets", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		application.handleAssets(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}

		var created assetView
		if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if created.Category != "阅读" {
			t.Fatalf("unexpected category: %s", created.Category)
		}
		if created.Icon != "📚" {
			t.Fatalf("unexpected icon: %s", created.Icon)
		}
		if created.SoldPrice == nil || *created.SoldPrice != 1200 {
			t.Fatalf("expected sold price, got %#v", created.SoldPrice)
		}
		if created.RecoveredRatio <= 0 {
			t.Fatalf("expected recovered ratio, got %f", created.RecoveredRatio)
		}

		updateBody := `{"name":"Kindle Paperwhite","icon":"📖","category":"阅读","price":1699,"purchaseDate":"2025-10-01","status":"active"}`
		updateReq := httptest.NewRequest(http.MethodPut, "/api/assets/"+strconv.FormatInt(created.ID, 10), strings.NewReader(updateBody))
		updateReq.Header.Set("Content-Type", "application/json")
		updateRec := httptest.NewRecorder()

		application.handleAssetByID(updateRec, updateReq)

		if updateRec.Code != http.StatusOK {
			t.Fatalf("unexpected update status: %d body=%s", updateRec.Code, updateRec.Body.String())
		}

		var updated assetView
		if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
			t.Fatalf("decode update response: %v", err)
		}
		if updated.Name != "Kindle Paperwhite" || updated.Status != statusActive {
			t.Fatalf("unexpected updated asset: %+v", updated)
		}
		if updated.Icon != "📖" {
			t.Fatalf("unexpected updated icon: %s", updated.Icon)
		}
		if updated.SoldPrice != nil || updated.SoldDate != nil {
			t.Fatal("expected sold fields to be cleared when asset becomes active")
		}

		deleteReq := httptest.NewRequest(http.MethodDelete, "/api/assets/"+strconv.FormatInt(created.ID, 10), nil)
		deleteRec := httptest.NewRecorder()
		application.handleAssetByID(deleteRec, deleteReq)

		if deleteRec.Code != http.StatusOK {
			t.Fatalf("unexpected delete status: %d body=%s", deleteRec.Code, deleteRec.Body.String())
		}
	})

	t.Run("sold asset without sold date is rejected", func(t *testing.T) {
		body := `{"name":"投影仪","category":"家电","price":2100,"purchaseDate":"2025-08-01","status":"sold","soldPrice":1200}`
		req := httptest.NewRequest(http.MethodPost, "/api/assets", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		application.handleAssets(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected bad request, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}
