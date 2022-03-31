// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package admin_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"storj.io/common/testcontext"
	"storj.io/common/testrand"
	"storj.io/storj/private/testplanet"
	"storj.io/storj/satellite"
	"storj.io/storj/satellite/oidc"
)

func TestAccountManagementAPIKeys(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   1,
		StorageNodeCount: 0,
		UplinkCount:      1,
		Reconfigure: testplanet.Reconfigure{
			Satellite: func(_ *zap.Logger, _ int, config *satellite.Config) {
				config.Admin.Address = "127.0.0.1:0"
			},
		},
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		address := planet.Satellites[0].Admin.Admin.Listener.Addr()
		satellite := planet.Satellites[0]
		keyService := satellite.API.AccountManagementAPIKeys.Service

		user, err := planet.Satellites[0].DB.Console().Users().GetByEmail(ctx, planet.Uplinks[0].Projects[0].Owner.Email)
		require.NoError(t, err)

		t.Run("create with default expiration", func(t *testing.T) {
			body := strings.NewReader(`{"expiration":""}`)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://"+address.String()+"/api/accountmanagementapikeys/%s", user.Email), body)
			require.NoError(t, err)
			req.Header.Set("Authorization", satellite.Config.Console.AuthToken)

			// get current time to check against ExpiresAt
			now := time.Now()

			response, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "application/json", response.Header.Get("Content-Type"))

			responseBody, err := ioutil.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			var output struct {
				APIKey    string    `json:"apikey"`
				ExpiresAt time.Time `json:"expiresAt"`
			}

			err = json.Unmarshal(responseBody, &output)
			require.NoError(t, err)

			userID, err := keyService.GetUserFromKey(ctx, output.APIKey)
			require.NoError(t, err)
			require.Equal(t, user.ID, userID)

			// check the expiration is around the time we expect
			defaultExpiration := satellite.Config.AccountManagementAPIKeys.DefaultExpiration
			require.True(t, output.ExpiresAt.After(now.Add(defaultExpiration)))
			require.True(t, output.ExpiresAt.Before(now.Add(defaultExpiration+time.Hour)))
		})

		t.Run("create with custom expiration", func(t *testing.T) {
			durationString := "3h"
			body := strings.NewReader(fmt.Sprintf(`{"expiration":"%s"}`, durationString))
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://"+address.String()+"/api/accountmanagementapikeys/%s", user.Email), body)
			require.NoError(t, err)
			req.Header.Set("Authorization", satellite.Config.Console.AuthToken)

			// get current time to check against ExpiresAt
			now := time.Now()

			response, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "application/json", response.Header.Get("Content-Type"))

			responseBody, err := ioutil.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			var output struct {
				APIKey    string    `json:"apikey"`
				ExpiresAt time.Time `json:"expiresAt"`
			}

			err = json.Unmarshal(responseBody, &output)
			require.NoError(t, err)

			userID, err := keyService.GetUserFromKey(ctx, output.APIKey)
			require.NoError(t, err)
			require.Equal(t, user.ID, userID)

			// check the expiration is around the time we expect
			durationTime, err := time.ParseDuration(durationString)
			require.NoError(t, err)
			require.True(t, output.ExpiresAt.After(now.Add(durationTime)))
			require.True(t, output.ExpiresAt.Before(now.Add(durationTime+time.Hour)))
		})

		t.Run("revoke key", func(t *testing.T) {
			apiKey := testrand.UUID().String()
			hash, err := keyService.HashKey(ctx, apiKey)
			require.NoError(t, err)

			expiresAt, err := keyService.InsertIntoDB(ctx, oidc.OAuthToken{
				UserID: user.ID,
				Kind:   oidc.KindAccountManagementTokenV0,
				Token:  hash,
			}, time.Now(), time.Hour)
			require.NoError(t, err)
			require.False(t, expiresAt.IsZero())

			req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("http://"+address.String()+"/api/accountmanagementapikeys/%s/revoke", apiKey), nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", satellite.Config.Console.AuthToken)

			response, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.NoError(t, response.Body.Close())

			_, err = keyService.GetUserFromKey(ctx, apiKey)
			require.Error(t, err)
		})
	})
}