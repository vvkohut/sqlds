package sqlds_test

import (
	"context"
	"os"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/hydrolix/sqlds/v5"
	"github.com/hydrolix/sqlds/v5/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDriver struct {
	sqlds.SQLMock
	rowLimit int64
}

func (d *mockDriver) Settings(ctx context.Context, settings backend.DataSourceInstanceSettings) sqlds.DriverSettings {
	ds := d.SQLMock.Settings(ctx, settings)
	ds.RowLimit = d.rowLimit
	return ds
}

func getMockGrafanaCfg(rowLimit string) *backend.GrafanaCfg {
	// needs all these properties to be set to avoid errors
	return backend.NewGrafanaCfg(map[string]string{
		"GF_SQL_ROW_LIMIT":                         rowLimit,
		"GF_SQL_MAX_OPEN_CONNS_DEFAULT":            "10",
		"GF_SQL_MAX_IDLE_CONNS_DEFAULT":            "5",
		"GF_SQL_MAX_CONN_LIFETIME_SECONDS_DEFAULT": "3600",
	})
}
func TestRowLimitFromConfig(t *testing.T) {
	// Create a mock config using the proper API
	mockConfig := getMockGrafanaCfg("200")

	// Create context with config
	ctx := backend.WithGrafanaConfig(context.Background(), mockConfig)

	// Create datasource with row limit enabled
	driver := &mockDriver{}
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-config", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, ctx, true)

	assert.Equal(t, int64(200), ds.GetRowLimit())
}

func TestRowLimitFromDriverSettings(t *testing.T) {
	// Create datasource with driver that has row limit
	driver := &mockDriver{rowLimit: 300}
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-driver", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, context.Background(), true)

	assert.Equal(t, int64(300), ds.GetRowLimit())
}

func TestRowLimitPrecedence(t *testing.T) {
	// Create a mock config using the proper API
	mockConfig := getMockGrafanaCfg("200")

	// Create context with config
	ctx := backend.WithGrafanaConfig(context.Background(), mockConfig)

	// Create datasource with driver that has row limit
	driver := &mockDriver{rowLimit: 300}
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-precedence", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, ctx, true)

	assert.Equal(t, int64(300), ds.GetRowLimit())
}

func TestRowLimitDisabled(t *testing.T) {
	// Create a mock config using the proper API
	mockConfig := getMockGrafanaCfg("200")
	// Create context with config
	ctx := backend.WithGrafanaConfig(context.Background(), mockConfig)

	// Create datasource with row limit disabled
	driver := &mockDriver{}
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-disabled", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, ctx, false)

	assert.Equal(t, int64(-1), ds.GetRowLimit())
}

func TestRowLimitDefault(t *testing.T) {
	// Create a mock config using the proper API
	mockConfig := backend.NewGrafanaCfg(map[string]string{})

	// Create context with config
	ctx := backend.WithGrafanaConfig(context.Background(), mockConfig)

	// Create datasource with row limit disabled
	driver := &mockDriver{}
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-disabled", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, ctx, false)

	assert.Equal(t, int64(-1), ds.GetRowLimit())
}

func TestSetDefaultRowLimit(t *testing.T) {
	driver := &mockDriver{}
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-set", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, context.Background(), false)

	ds.SetDefaultRowLimit(500)

	assert.Equal(t, int64(500), ds.GetRowLimit())
	assert.True(t, ds.EnableRowLimit)
}

func TestRowLimitPassedToQuery(t *testing.T) {
	// Set up test data
	testData := test.Data{
		Cols: []test.Column{
			{Name: "id", DataType: "INTEGER", Kind: int64(0)},
			{Name: "name", DataType: "TEXT", Kind: ""},
		},
		Rows: [][]any{
			{int64(1), "test1"},
			{int64(2), "test2"},
			{int64(3), "test3"},
		},
	}

	// Create datasource with row limit
	driver, _ := test.NewDriver("rowlimit-query", testData, nil, test.DriverOpts{})
	settings := backend.DataSourceInstanceSettings{UID: "rowlimit-query", JSONData: []byte("{}")}
	ds := newRowLimitTestDatasource(t, driver, settings, context.Background(), false)
	ds.SetDefaultRowLimit(2)

	// Create query request
	req := &backend.QueryDataRequest{
		PluginContext: backend.PluginContext{
			DataSourceInstanceSettings: &settings,
		},
		Queries: []backend.DataQuery{
			{
				RefID: "A",
				JSON:  []byte(`{"rawSql": "SELECT * FROM test"}`),
			},
		},
	}

	// Execute query
	resp, err := ds.QueryData(context.Background(), req)
	assert.NoError(t, err)

	// Verify response
	queryResp := resp.Responses["A"]
	assert.NoError(t, queryResp.Error)
	assert.NotNil(t, queryResp.Frames)
	assert.Len(t, queryResp.Frames, 1)

	// Verify row limit was applied (should only have 2 rows)
	frame := queryResp.Frames[0]
	rowCount, _ := frame.RowLen()
	assert.Equal(t, 2, rowCount)
}

func TestRowLimitFromEnvVar(t *testing.T) {
	// Save original env var value to restore later
	originalValue, originalExists := os.LookupEnv("GF_DATAPROXY_ROW_LIMIT")

	// Clean up after test
	defer func() {
		if originalExists {
			os.Setenv("GF_DATAPROXY_ROW_LIMIT", originalValue)
		} else {
			os.Unsetenv("GF_DATAPROXY_ROW_LIMIT")
		}
	}()

	tests := []struct {
		name           string
		envValue       string
		expectedLimit  int64
		configValue    string
		driverRowLimit int64
	}{
		{
			name:          "valid env var",
			envValue:      "400",
			expectedLimit: 400,
		},
		{
			name:          "invalid env var",
			envValue:      "not-a-number",
			expectedLimit: -1,
		},
		{
			name:          "negative env var",
			envValue:      "-10",
			expectedLimit: -1,
		},
		{
			name:          "env var precedence over config",
			envValue:      "400",
			configValue:   "200",
			expectedLimit: 400,
		},
		{
			name:           "driver settings precedence over env var",
			envValue:       "400",
			driverRowLimit: 300,
			expectedLimit:  300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GF_DATAPROXY_ROW_LIMIT", tt.envValue)

			ctx := context.Background()
			if tt.configValue != "" {
				mockConfig := getMockGrafanaCfg(tt.configValue)
				ctx = backend.WithGrafanaConfig(ctx, mockConfig)
			}

			driver := &mockDriver{rowLimit: tt.driverRowLimit}
			settings := backend.DataSourceInstanceSettings{UID: "rowlimit-env-" + tt.name, JSONData: []byte("{}")}
			ds := newRowLimitTestDatasource(t, driver, settings, ctx, true)

			assert.Equal(t, tt.expectedLimit, ds.GetRowLimit())
		})
	}
}

// validTestSettings returns DataSourceInstanceSettings with minimal valid plugin JSON
// to pass NewPluginSettings validation (host, port, protocol are required).
func validTestSettings(uid string) backend.DataSourceInstanceSettings {
	return backend.DataSourceInstanceSettings{
		UID:      uid,
		JSONData: []byte(`{"host":"localhost","port":9000,"protocol":"native"}`),
	}
}

// newRowLimitTestDatasource creates a HydrolixDatasource for rowlimit testing.
func newRowLimitTestDatasource(t *testing.T, driver sqlds.Driver, settings backend.DataSourceInstanceSettings, ctx context.Context, enableRowLimit bool) *sqlds.HydrolixDatasource {
	// Use valid plugin settings for NewConnector, but keep original settings for DriverSettings
	connSettings := validTestSettings(settings.UID)
	_, err := driver.Connect(context.Background(), connSettings, nil)
	require.NoError(t, err)
	conn, err := sqlds.NewConnector(context.Background(), driver, connSettings)
	require.NoError(t, err)

	ds := &sqlds.HydrolixDatasource{Connector: conn, EnableRowLimit: enableRowLimit}
	_, err = ds.NewDatasource(ctx, settings)
	require.NoError(t, err)

	return ds
}
