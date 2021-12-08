package service

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.etcd.io/bbolt"
)

func openCacheDB(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{})
	if err != nil {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("failed to remove corrupted database: %w", err)
		}
		if db, err = bbolt.Open(path, 0600, &bbolt.Options{}); err != nil {
			return nil, err
		}
	}
	return db, nil
}

type remoteConfigKey struct {
	orgID      int64
	appKey     string
	datacenter string
}

func parseRemoteConfigKey(rawKey string) (remoteConfigKey, error) {
	split := strings.SplitN(rawKey, "/", 3)
	if len(split) < 3 {
		return remoteConfigKey{}, fmt.Errorf("invalid remote configuration key format, should be datacenter/org_id/app_key")
	}
	orgID, err := strconv.ParseInt(split[1], 10, 64)
	if err != nil {
		return remoteConfigKey{}, err
	}
	return remoteConfigKey{
		orgID:      orgID,
		appKey:     split[2],
		datacenter: split[0],
	}, nil
}
