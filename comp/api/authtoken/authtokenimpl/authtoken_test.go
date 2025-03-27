// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package authtokenimpl

// func TestAuthToken(t *testing.T) {
// 	optAuth := fxutil.Test[option.Option[authtoken.Component]](t,
// 		Module(),
// 		fx.Provide(func() log.Component { return logmock.New(t) }),
// 		config.MockModule(),
// 	)

// 	_, ok := optAuth.Get()
// 	require.False(t, ok)
// 	require.NotNil(t, optAuth)

// auth := fxutil.Test[authtoken.Component](t,
// 	Module(),
// 	fx.Provide(func() log.Component { return logmock.New(t) }),
// 	config.MockModule(),
// )

// require.NotNil(t, auth)
// }

// func TestGet(t *testing.T) {
// 	dir := t.TempDir()
// 	authPath := filepath.Join(dir, "auth_token")
// 	var cfg config.Component
// 	overrides := map[string]any{
// 		"auth_token_file_path": authPath,
// 	}

// 	comp := newAuthToken(
// 		fxutil.Test[dependencies](
// 			t,
// 			fx.Provide(func() log.Component { return logmock.New(t) }),
// 			config.MockModule(),
// 			fx.Populate(&cfg),
// 			fx.Replace(config.MockParams{Overrides: overrides}),
// 		),
// 	).(*authToken)

// 	comp.Get()

// 	err := os.WriteFile(authPath, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0777)
// 	require.NoError(t, err)

// 	// Should be empty because the cert/key weren't generated yet
// 	comp.Get()
// 	assert.False(t, comp.tokenLoaded)

// 	// generating IPC cert/key files
// 	_, _, err = cert.FetchOrCreateIPCCert(context.Background(), cfg)
// 	require.NoError(t, err)

// 	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", comp.Get())
// 	assert.True(t, comp.tokenLoaded)

// }
