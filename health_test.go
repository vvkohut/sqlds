package sqlds

import (
	"context"
	"database/sql"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getFakeConnector(t *testing.T, shouldFail bool) Connector {
	t.Helper()
	db, _ := newSqlmockDB(t)
	if shouldFail {
		db.Close()
	}

	driver := &stubDriver{
		settings:   DriverSettings{},
		connectDBs: []*sql.DB{db},
	}
	c, _ := NewConnector(context.TODO(), driver, buildInstanceSettings())
	return c
}

func TestHealthChecker_Check(t *testing.T) {
	tests := []struct {
		name            string
		Connector       Connector
		Metrics         Metrics
		PreCheckHealth  func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult
		PostCheckHealth func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult
		ctx             context.Context
		req             *backend.CheckHealthRequest
		want            *backend.CheckHealthResult
		wantErr         error
	}{
		{
			name:      "default health check should return valid result",
			Connector: getFakeConnector(t, false),
		},
		{
			name:      "should not error when pre check succeed",
			Connector: getFakeConnector(t, false),
			PreCheckHealth: func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
				return &backend.CheckHealthResult{Status: backend.HealthStatusOk}
			},
		},
		{
			name:      "should error when pre check failed",
			Connector: getFakeConnector(t, false),
			PreCheckHealth: func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
				return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "unknown error"}
			},
			want: &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "unknown error"},
		},
		{
			name:      "should return actual error when pre and post health check succeed but actual connect failed",
			Connector: getFakeConnector(t, true),
			PreCheckHealth: func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
				return &backend.CheckHealthResult{Status: backend.HealthStatusOk}
			},
			PostCheckHealth: func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
				return &backend.CheckHealthResult{Status: backend.HealthStatusOk}
			},
			want: &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "sql: database is closed"},
		},
		{
			name:      "should not error when post check succeed",
			Connector: getFakeConnector(t, false),
			PostCheckHealth: func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
				return &backend.CheckHealthResult{Status: backend.HealthStatusOk}
			},
		},
		{
			name:      "should error when post check failed",
			Connector: getFakeConnector(t, false),
			PostCheckHealth: func(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
				return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "unknown error"}
			},
			want: &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "unknown error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := tt.Connector
			if connector == nil {
				connector = getFakeConnector(t, false)
			}
			req := tt.req
			if req == nil {
				req = &backend.CheckHealthRequest{}
			}
			want := tt.want
			if want == nil {
				want = &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Data source is working"}
			}
			hc := &HealthChecker{
				Connector:       connector,
				Metrics:         tt.Metrics,
				PreCheckHealth:  tt.PreCheckHealth,
				PostCheckHealth: tt.PostCheckHealth,
			}
			got, err := hc.Check(tt.ctx, req)
			if tt.wantErr != nil {
				require.NotNil(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
				return
			}
			require.Nil(t, err)
			require.NotNil(t, got)
			assert.Equal(t, want.Message, got.Message)
			assert.Equal(t, want.Status, got.Status)
		})
	}
}
