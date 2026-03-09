package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const dateLayout = "2006-01-02"

type assetStatus string

const (
	statusActive assetStatus = "active"
	statusIdle   assetStatus = "idle"
	statusSold   assetStatus = "sold"
)

var validStatuses = []assetStatus{statusActive, statusIdle, statusSold}

type asset struct {
	ID              int64       `json:"id"`
	Name            string      `json:"name"`
	Icon            string      `json:"icon"`
	Category        string      `json:"category"`
	Price           float64     `json:"price"`
	PurchaseDate    string      `json:"purchaseDate"`
	ExpectedYears   *float64    `json:"expectedYears,omitempty"`
	TargetDailyCost *float64    `json:"targetDailyCost,omitempty"`
	Status          assetStatus `json:"status"`
	SoldPrice       *float64    `json:"soldPrice,omitempty"`
	SoldDate        *string     `json:"soldDate,omitempty"`
	CreatedAt       string      `json:"createdAt"`
	UpdatedAt       string      `json:"updatedAt"`
}

type assetView struct {
	asset
	DaysUsed              int     `json:"daysUsed"`
	DailyCost             float64 `json:"dailyCost"`
	RecoveredRatio        float64 `json:"recoveredRatio"`
	LifecycleProgress     float64 `json:"lifecycleProgress"`
	LifecyclePercentLabel string  `json:"lifecyclePercentLabel"`
}

type categoryStat struct {
	Name       string  `json:"name"`
	Count      int     `json:"count"`
	TotalValue float64 `json:"totalValue"`
	AverageDay float64 `json:"averageDay"`
}

type monthlyTrend struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type analyticsView struct {
	Categories   []categoryStat `json:"categories"`
	StatusValues []categoryStat `json:"statusValues"`
	MonthlySpend []monthlyTrend `json:"monthlySpend"`
}

type summaryView struct {
	TotalAssetValue float64            `json:"totalAssetValue"`
	AverageDaily    float64            `json:"averageDaily"`
	Counts          map[string]int     `json:"counts"`
	StatusValue     map[string]float64 `json:"statusValue"`
}

type dashboardResponse struct {
	Summary   summaryView   `json:"summary"`
	Analytics analyticsView `json:"analytics"`
	Assets    []assetView   `json:"assets"`
}

type assetPayload struct {
	Name            string   `json:"name"`
	Icon            string   `json:"icon"`
	Category        string   `json:"category"`
	Price           float64  `json:"price"`
	PurchaseDate    string   `json:"purchaseDate"`
	ExpectedYears   *float64 `json:"expectedYears"`
	TargetDailyCost *float64 `json:"targetDailyCost"`
	Status          string   `json:"status"`
	SoldPrice       *float64 `json:"soldPrice"`
	SoldDate        *string  `json:"soldDate"`
}

type statusUpdateRequest struct {
	Status string `json:"status"`
}

type app struct {
	db *sql.DB
}

func main() {
	ctx := context.Background()

	db, err := sql.Open("sqlite", "file:assets.db?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := initSchema(ctx, db); err != nil {
		log.Fatalf("init schema: %v", err)
	}
	if err := seedAssets(ctx, db); err != nil {
		log.Fatalf("seed assets: %v", err)
	}

	application := &app{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dashboard", application.handleDashboard)
	mux.HandleFunc("/api/assets", application.handleAssets)
	mux.HandleFunc("/api/assets/", application.handleAssetByID)

	staticDir := filepath.Join(".", "web")
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	addr := ":8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		addr = ":" + fromEnv
	}

	log.Printf("asset dashboard listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

func initSchema(ctx context.Context, db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS assets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		icon TEXT NOT NULL DEFAULT '📦',
		category TEXT NOT NULL DEFAULT '未分类',
		price REAL NOT NULL CHECK(price >= 0),
		purchase_date TEXT NOT NULL,
		expected_years REAL,
		target_daily_cost REAL,
		status TEXT NOT NULL CHECK(status IN ('active', 'idle', 'sold')),
		sold_price REAL,
		sold_date TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return err
	}

	requiredColumns := map[string]string{
		"icon":       "ALTER TABLE assets ADD COLUMN icon TEXT NOT NULL DEFAULT '📦'",
		"category":   "ALTER TABLE assets ADD COLUMN category TEXT NOT NULL DEFAULT '未分类'",
		"sold_price": "ALTER TABLE assets ADD COLUMN sold_price REAL",
		"sold_date":  "ALTER TABLE assets ADD COLUMN sold_date TEXT",
		"updated_at": "ALTER TABLE assets ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''",
	}
	existing, err := readColumns(ctx, db, "assets")
	if err != nil {
		return err
	}
	for name, statement := range requiredColumns {
		if !existing[name] {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}

	_, err = db.ExecContext(ctx, `UPDATE assets SET updated_at = created_at WHERE updated_at = '' OR updated_at IS NULL`)
	return err
}

func readColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func seedAssets(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().Format(time.RFC3339)
	soldDate := daysAgo(200)
	samples := []asset{
		{Name: "NanoPi M6", Icon: "📦", Category: "数码", Price: 650, PurchaseDate: daysAgo(76), TargetDailyCost: floatPtr(7.5), Status: statusActive, CreatedAt: now, UpdatedAt: now},
		{Name: "iPad Pro M2", Icon: "📱", Category: "数码", Price: 4640, PurchaseDate: daysAgo(257), ExpectedYears: floatPtr(4), Status: statusActive, CreatedAt: now, UpdatedAt: now},
		{Name: "MacBook Air M4", Icon: "💻", Category: "办公", Price: 4669, PurchaseDate: daysAgo(90), ExpectedYears: floatPtr(5), Status: statusActive, CreatedAt: now, UpdatedAt: now},
		{Name: "西数 8TB 硬盘", Icon: "💾", Category: "存储", Price: 892, PurchaseDate: daysAgo(509), TargetDailyCost: floatPtr(2), Status: statusActive, CreatedAt: now, UpdatedAt: now},
		{Name: "显示器支架", Icon: "🪑", Category: "家居", Price: 299, PurchaseDate: daysAgo(412), ExpectedYears: floatPtr(6), Status: statusIdle, CreatedAt: now, UpdatedAt: now},
		{Name: "旧相机", Icon: "📷", Category: "影像", Price: 3200, PurchaseDate: daysAgo(860), ExpectedYears: floatPtr(5), Status: statusSold, SoldPrice: floatPtr(1600), SoldDate: &soldDate, CreatedAt: now, UpdatedAt: now},
	}

	stmt, err := db.PrepareContext(ctx, `
		INSERT INTO assets (
			name, icon, category, price, purchase_date, expected_years, target_daily_cost, status, sold_price, sold_date, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sample := range samples {
		if _, err := stmt.ExecContext(ctx,
			sample.Name,
			sample.Icon,
			sample.Category,
			sample.Price,
			sample.PurchaseDate,
			sample.ExpectedYears,
			sample.TargetDailyCost,
			sample.Status,
			sample.SoldPrice,
			sample.SoldDate,
			sample.CreatedAt,
			sample.UpdatedAt,
		); err != nil {
			return err
		}
	}

	return nil
}

func (a *app) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	assets, err := a.listAssetViews(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, dashboardResponse{
		Summary:   buildSummary(assets),
		Analytics: buildAnalytics(assets),
		Assets:    assets,
	})
}

func (a *app) handleAssets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		assets, err := a.listAssetViews(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, assets)
	case http.MethodPost:
		var input assetPayload
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
			return
		}

		created, err := a.createAsset(r.Context(), input)
		if err != nil {
			writeValidationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w)
	}
}

func (a *app) handleAssetByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/assets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid asset id"))
		return
	}

	if len(parts) == 2 && parts[1] == "status" {
		if r.Method != http.MethodPatch {
			methodNotAllowed(w)
			return
		}
		var input statusUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
			return
		}
		view, err := a.updateStatus(r.Context(), id, input.Status)
		if err != nil {
			writeValidationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}

	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var input assetPayload
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
			return
		}
		view, err := a.updateAsset(r.Context(), id, input)
		if err != nil {
			writeValidationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		if err := a.deleteAsset(r.Context(), id); err != nil {
			writeValidationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		methodNotAllowed(w)
	}
}

var errBadRequest = errors.New("bad request")

func (a *app) createAsset(ctx context.Context, input assetPayload) (assetView, error) {
	params, err := normalizePayload(input)
	if err != nil {
		return assetView{}, err
	}

	now := time.Now().Format(time.RFC3339)
	result, err := a.db.ExecContext(ctx, `
		INSERT INTO assets (
			name, icon, category, price, purchase_date, expected_years, target_daily_cost, status, sold_price, sold_date, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.Name, params.Icon, params.Category, params.Price, params.PurchaseDate, params.ExpectedYears, params.TargetDailyCost, params.Status, params.SoldPrice, params.SoldDate, now, now)
	if err != nil {
		return assetView{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return assetView{}, err
	}
	return a.getAssetView(ctx, id)
}

func (a *app) updateAsset(ctx context.Context, id int64, input assetPayload) (assetView, error) {
	params, err := normalizePayload(input)
	if err != nil {
		return assetView{}, err
	}

	result, err := a.db.ExecContext(ctx, `
		UPDATE assets
		SET name = ?, icon = ?, category = ?, price = ?, purchase_date = ?, expected_years = ?, target_daily_cost = ?, status = ?, sold_price = ?, sold_date = ?, updated_at = ?
		WHERE id = ?
	`, params.Name, params.Icon, params.Category, params.Price, params.PurchaseDate, params.ExpectedYears, params.TargetDailyCost, params.Status, params.SoldPrice, params.SoldDate, time.Now().Format(time.RFC3339), id)
	if err != nil {
		return assetView{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return assetView{}, err
	}
	if rows == 0 {
		return assetView{}, sql.ErrNoRows
	}
	return a.getAssetView(ctx, id)
}

func (a *app) updateStatus(ctx context.Context, id int64, status string) (assetView, error) {
	next := normalizeStatus(status)
	if next == "" {
		return assetView{}, fmt.Errorf("%w: unsupported status", errBadRequest)
	}

	current, err := a.getAsset(ctx, id)
	if err != nil {
		return assetView{}, err
	}
	current.Status = next
	if next != statusSold {
		current.SoldPrice = nil
		current.SoldDate = nil
	}
	return a.updateAsset(ctx, id, assetPayload{
		Name:            current.Name,
		Icon:            current.Icon,
		Category:        current.Category,
		Price:           current.Price,
		PurchaseDate:    current.PurchaseDate,
		ExpectedYears:   current.ExpectedYears,
		TargetDailyCost: current.TargetDailyCost,
		Status:          string(current.Status),
		SoldPrice:       current.SoldPrice,
		SoldDate:        current.SoldDate,
	})
}

func (a *app) deleteAsset(ctx context.Context, id int64) error {
	result, err := a.db.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *app) getAsset(ctx context.Context, id int64) (asset, error) {
	row := a.db.QueryRowContext(ctx, `
		SELECT id, name, icon, category, price, purchase_date, expected_years, target_daily_cost, status, sold_price, sold_date, created_at, updated_at
		FROM assets WHERE id = ?
	`, id)
	return scanAsset(row)
}

func (a *app) getAssetView(ctx context.Context, id int64) (assetView, error) {
	record, err := a.getAsset(ctx, id)
	if err != nil {
		return assetView{}, err
	}
	return buildAssetView(record)
}

func (a *app) listAssetViews(ctx context.Context) ([]assetView, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, name, icon, category, price, purchase_date, expected_years, target_daily_cost, status, sold_price, sold_date, created_at, updated_at
		FROM assets
		ORDER BY purchase_date DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []assetView
	for rows.Next() {
		record, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		view, err := buildAssetView(record)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAsset(scan scanner) (asset, error) {
	var record asset
	err := scan.Scan(
		&record.ID,
		&record.Name,
		&record.Icon,
		&record.Category,
		&record.Price,
		&record.PurchaseDate,
		&record.ExpectedYears,
		&record.TargetDailyCost,
		&record.Status,
		&record.SoldPrice,
		&record.SoldDate,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	return record, err
}

func buildAssetView(record asset) (assetView, error) {
	purchaseDate, err := time.Parse(dateLayout, record.PurchaseDate)
	if err != nil {
		return assetView{}, err
	}
	daysUsed := int(time.Since(purchaseDate).Hours()/24) + 1
	if daysUsed < 1 {
		daysUsed = 1
	}

	dailyCost := round2(record.Price / float64(daysUsed))
	expectedDays := 0.0
	switch {
	case record.ExpectedYears != nil:
		expectedDays = *record.ExpectedYears * 365
	case record.TargetDailyCost != nil && *record.TargetDailyCost > 0:
		expectedDays = record.Price / *record.TargetDailyCost
	}

	progress := 0.0
	if expectedDays > 0 {
		progress = math.Min(100, round2((float64(daysUsed)/expectedDays)*100))
	}

	recoveredRatio := 0.0
	if record.SoldPrice != nil && record.Price > 0 {
		recoveredRatio = math.Min(100, round2((*record.SoldPrice/record.Price)*100))
	}

	return assetView{
		asset:                 record,
		DaysUsed:              daysUsed,
		DailyCost:             dailyCost,
		RecoveredRatio:        recoveredRatio,
		LifecycleProgress:     progress,
		LifecyclePercentLabel: fmt.Sprintf("%.0f%%", progress),
	}, nil
}

func buildSummary(assets []assetView) summaryView {
	summary := summaryView{
		Counts: map[string]int{
			string(statusActive): 0,
			string(statusIdle):   0,
			string(statusSold):   0,
		},
		StatusValue: map[string]float64{
			string(statusActive): 0,
			string(statusIdle):   0,
			string(statusSold):   0,
		},
	}

	if len(assets) == 0 {
		return summary
	}

	totalDaily := 0.0
	for _, item := range assets {
		summary.TotalAssetValue += item.Price
		totalDaily += item.DailyCost
		summary.Counts[string(item.Status)]++
		summary.StatusValue[string(item.Status)] += item.Price
	}

	summary.TotalAssetValue = round2(summary.TotalAssetValue)
	summary.AverageDaily = round2(totalDaily)
	for key, value := range summary.StatusValue {
		summary.StatusValue[key] = round2(value)
	}

	return summary
}

func buildAnalytics(assets []assetView) analyticsView {
	categoryIndex := map[string]*categoryStat{}
	statusStats := map[string]*categoryStat{}
	monthlySpend := map[string]float64{}

	for _, status := range validStatuses {
		statusStats[string(status)] = &categoryStat{Name: string(status)}
	}

	for _, item := range assets {
		entry := categoryIndex[item.Category]
		if entry == nil {
			entry = &categoryStat{Name: item.Category}
			categoryIndex[item.Category] = entry
		}
		entry.Count++
		entry.TotalValue += item.Price
		entry.AverageDay += item.DailyCost

		statusEntry := statusStats[string(item.Status)]
		statusEntry.Count++
		statusEntry.TotalValue += item.Price
		statusEntry.AverageDay += item.DailyCost

		monthLabel := item.PurchaseDate[:7]
		monthlySpend[monthLabel] += item.Price
	}

	categories := make([]categoryStat, 0, len(categoryIndex))
	for _, stat := range categoryIndex {
		if stat.Count > 0 {
			stat.TotalValue = round2(stat.TotalValue)
			stat.AverageDay = round2(stat.AverageDay / float64(stat.Count))
		}
		categories = append(categories, *stat)
	}
	slices.SortFunc(categories, func(a, b categoryStat) int {
		switch {
		case b.TotalValue > a.TotalValue:
			return 1
		case b.TotalValue < a.TotalValue:
			return -1
		default:
			return strings.Compare(a.Name, b.Name)
		}
	})

	statusValues := make([]categoryStat, 0, len(statusStats))
	for _, status := range validStatuses {
		stat := statusStats[string(status)]
		if stat.Count > 0 {
			stat.TotalValue = round2(stat.TotalValue)
			stat.AverageDay = round2(stat.AverageDay / float64(stat.Count))
		}
		statusValues = append(statusValues, *stat)
	}

	months := make([]string, 0, len(monthlySpend))
	for label := range monthlySpend {
		months = append(months, label)
	}
	slices.Sort(months)
	trends := make([]monthlyTrend, 0, len(months))
	for _, label := range months {
		trends = append(trends, monthlyTrend{
			Label: label,
			Value: round2(monthlySpend[label]),
		})
	}

	return analyticsView{
		Categories:   categories,
		StatusValues: statusValues,
		MonthlySpend: trends,
	}
}

func normalizePayload(input assetPayload) (assetPayload, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Icon = normalizeIcon(input.Icon)
	input.Category = strings.TrimSpace(input.Category)
	status := normalizeStatus(input.Status)
	if input.Category == "" {
		input.Category = "未分类"
	}

	if input.Name == "" || input.Price <= 0 || status == "" {
		return assetPayload{}, fmt.Errorf("%w: name, price and status are required", errBadRequest)
	}
	purchaseDate, err := time.Parse(dateLayout, input.PurchaseDate)
	if err != nil {
		return assetPayload{}, fmt.Errorf("%w: purchaseDate must be YYYY-MM-DD", errBadRequest)
	}
	if input.ExpectedYears != nil && *input.ExpectedYears <= 0 {
		return assetPayload{}, fmt.Errorf("%w: expectedYears must be greater than 0", errBadRequest)
	}
	if input.TargetDailyCost != nil && *input.TargetDailyCost <= 0 {
		return assetPayload{}, fmt.Errorf("%w: targetDailyCost must be greater than 0", errBadRequest)
	}

	input.Status = string(status)
	input.PurchaseDate = purchaseDate.Format(dateLayout)

	if status == statusSold {
		if input.SoldPrice == nil || *input.SoldPrice < 0 {
			return assetPayload{}, fmt.Errorf("%w: soldPrice is required for sold assets", errBadRequest)
		}
		if input.SoldDate == nil || strings.TrimSpace(*input.SoldDate) == "" {
			return assetPayload{}, fmt.Errorf("%w: soldDate is required for sold assets", errBadRequest)
		}
		soldDate, err := time.Parse(dateLayout, strings.TrimSpace(*input.SoldDate))
		if err != nil {
			return assetPayload{}, fmt.Errorf("%w: soldDate must be YYYY-MM-DD", errBadRequest)
		}
		if soldDate.Before(purchaseDate) {
			return assetPayload{}, fmt.Errorf("%w: soldDate must not be before purchaseDate", errBadRequest)
		}
		normalizedSoldDate := soldDate.Format(dateLayout)
		input.SoldDate = &normalizedSoldDate
	} else {
		input.SoldPrice = nil
		input.SoldDate = nil
	}

	return input, nil
}

func normalizeStatus(raw string) assetStatus {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(statusActive):
		return statusActive
	case string(statusIdle):
		return statusIdle
	case string(statusSold):
		return statusSold
	default:
		return ""
	}
}

func normalizeIcon(raw string) string {
	icon := strings.TrimSpace(raw)
	if icon == "" {
		return "📦"
	}
	runes := []rune(icon)
	if len(runes) > 2 {
		return string(runes[:2])
	}
	return string(runes)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeValidationError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errBadRequest):
		status = http.StatusBadRequest
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	}
	writeError(w, status, err)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func floatPtr(v float64) *float64 {
	return &v
}

func daysAgo(days int) string {
	return time.Now().AddDate(0, 0, -days).Format(dateLayout)
}
