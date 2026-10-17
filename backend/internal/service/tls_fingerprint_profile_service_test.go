package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

type mockTLSFPRepo struct {
	profiles map[int64]*model.TLSFingerprintProfile
}

func (m *mockTLSFPRepo) List(ctx context.Context) ([]*model.TLSFingerprintProfile, error) {
	var res []*model.TLSFingerprintProfile
	for _, p := range m.profiles {
		res = append(res, p)
	}
	return res, nil
}

func (m *mockTLSFPRepo) GetByID(ctx context.Context, id int64) (*model.TLSFingerprintProfile, error) {
	if p, ok := m.profiles[id]; ok {
		return p, nil
	}
	return nil, nil
}

func (m *mockTLSFPRepo) Create(ctx context.Context, profile *model.TLSFingerprintProfile) (*model.TLSFingerprintProfile, error) {
	return profile, nil
}

func (m *mockTLSFPRepo) Update(ctx context.Context, profile *model.TLSFingerprintProfile) (*model.TLSFingerprintProfile, error) {
	return profile, nil
}

func (m *mockTLSFPRepo) Delete(ctx context.Context, id int64) error {
	delete(m.profiles, id)
	return nil
}

func TestResolveTLSProfile(t *testing.T) {
	repo := &mockTLSFPRepo{
		profiles: map[int64]*model.TLSFingerprintProfile{
			100: {
				ID:           100,
				Name:         "Custom Bound Profile",
				EnableGREASE: true,
			},
		},
	}
	svc := NewTLSFingerprintProfileService(repo, nil)

	t.Run("TLS disabled on account returns nil", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}
		profile := svc.ResolveTLSProfile(acc)
		assert.Nil(t, profile)
	})

	t.Run("nil account returns nil", func(t *testing.T) {
		profile := svc.ResolveTLSProfile(nil)
		assert.Nil(t, profile)
	})

	t.Run("OpenAI OAuth with custom profile bound returns custom profile", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"tls_fingerprint_profile_id": float64(100),
			},
		}
		profile := svc.ResolveTLSProfile(acc)
		require.NotNil(t, profile)
		assert.Equal(t, "Custom Bound Profile", profile.Name)
		assert.True(t, profile.EnableGREASE)
	})

	t.Run("OpenAI OAuth with TLS enabled but no profile bound returns Codex CLI rustls profile", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
		}
		profile := svc.ResolveTLSProfile(acc)
		require.NotNil(t, profile)
		assert.Equal(t, "Codex CLI (rustls)", profile.Name)
		assert.False(t, profile.EnableGREASE)
		assert.Len(t, profile.CipherSuites, 9)
		assert.Len(t, profile.SignatureAlgorithms, 8)
		assert.Len(t, profile.Extensions, 9)
		assert.Equal(t, []uint16{0, 23, 10, 11, 13, 16, 43, 51, 45}, profile.Extensions)
	})

	t.Run("Anthropic OAuth account with TLS enabled but no profile bound returns Node default", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"enable_tls_fingerprint": true,
			},
		}
		profile := svc.ResolveTLSProfile(acc)
		require.NotNil(t, profile)
		assert.Equal(t, "Built-in Default (Node.js 24.x)", profile.Name)
	})
}
