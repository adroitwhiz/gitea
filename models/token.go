// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"crypto/subtle"
	"fmt"
	"time"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"

	gouuid "github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru"
)

var successfulAccessTokenCache *lru.Cache

// AccessToken represents a personal access token.
type AccessToken struct {
	ID             int64 `xorm:"pk autoincr"`
	UID            int64 `xorm:"INDEX"`
	Name           string
	Token          string `xorm:"-"`
	TokenHash      string `xorm:"UNIQUE"` // sha256 of token
	TokenSalt      string
	TokenLastEight string `xorm:"token_last_eight"`

	CreatedUnix       timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix       timeutil.TimeStamp `xorm:"INDEX updated"`
	HasRecentActivity bool               `xorm:"-"`
	HasUsed           bool               `xorm:"-"`
}

// AfterLoad is invoked from XORM after setting the values of all fields of this object.
func (t *AccessToken) AfterLoad() {
	t.HasUsed = t.UpdatedUnix > t.CreatedUnix
	t.HasRecentActivity = t.UpdatedUnix.AddDuration(7*24*time.Hour) > timeutil.TimeStampNow()
}

func init() {
	db.RegisterModel(new(AccessToken), func() error {
		if setting.SuccessfulTokensCacheSize > 0 {
			var err error
			successfulAccessTokenCache, err = lru.New(setting.SuccessfulTokensCacheSize)
			if err != nil {
				return fmt.Errorf("unable to allocate AccessToken cache: %v", err)
			}
		} else {
			successfulAccessTokenCache = nil
		}
		return nil
	})
}

// NewAccessToken creates new access token.
func NewAccessToken(t *AccessToken) error {
	salt, err := util.RandomString(10)
	if err != nil {
		return err
	}
	t.TokenSalt = salt
	t.Token = base.EncodeSha1(gouuid.New().String())
	t.TokenHash = hashToken(t.Token, t.TokenSalt)
	t.TokenLastEight = t.Token[len(t.Token)-8:]
	_, err = db.DefaultContext().Engine().Insert(t)
	return err
}

func getAccessTokenIDFromCache(token string) int64 {
	if successfulAccessTokenCache == nil {
		return 0
	}
	tInterface, ok := successfulAccessTokenCache.Get(token)
	if !ok {
		return 0
	}
	t, ok := tInterface.(int64)
	if !ok {
		return 0
	}
	return t
}

// GetAccessTokenBySHA returns access token by given token value
func GetAccessTokenBySHA(token string) (*AccessToken, error) {
	if token == "" {
		return nil, ErrAccessTokenEmpty{}
	}
	// A token is defined as being SHA1 sum these are 40 hexadecimal bytes long
	if len(token) != 40 {
		return nil, ErrAccessTokenNotExist{token}
	}
	for _, x := range []byte(token) {
		if x < '0' || (x > '9' && x < 'a') || x > 'f' {
			return nil, ErrAccessTokenNotExist{token}
		}
	}

	lastEight := token[len(token)-8:]

	if id := getAccessTokenIDFromCache(token); id > 0 {
		token := &AccessToken{
			TokenLastEight: lastEight,
		}
		// Re-get the token from the db in case it has been deleted in the intervening period
		has, err := db.DefaultContext().Engine().ID(id).Get(token)
		if err != nil {
			return nil, err
		}
		if has {
			return token, nil
		}
		successfulAccessTokenCache.Remove(token)
	}

	var tokens []AccessToken
	err := db.DefaultContext().Engine().Table(&AccessToken{}).Where("token_last_eight = ?", lastEight).Find(&tokens)
	if err != nil {
		return nil, err
	} else if len(tokens) == 0 {
		return nil, ErrAccessTokenNotExist{token}
	}

	for _, t := range tokens {
		tempHash := hashToken(token, t.TokenSalt)
		if subtle.ConstantTimeCompare([]byte(t.TokenHash), []byte(tempHash)) == 1 {
			if successfulAccessTokenCache != nil {
				successfulAccessTokenCache.Add(token, t.ID)
			}
			return &t, nil
		}
	}
	return nil, ErrAccessTokenNotExist{token}
}

// AccessTokenByNameExists checks if a token name has been used already by a user.
func AccessTokenByNameExists(token *AccessToken) (bool, error) {
	return db.DefaultContext().Engine().Table("access_token").Where("name = ?", token.Name).And("uid = ?", token.UID).Exist()
}

// ListAccessTokensOptions contain filter options
type ListAccessTokensOptions struct {
	ListOptions
	Name   string
	UserID int64
}

// ListAccessTokens returns a list of access tokens belongs to given user.
func ListAccessTokens(opts ListAccessTokensOptions) ([]*AccessToken, error) {
	sess := db.DefaultContext().Engine().Where("uid=?", opts.UserID)

	if len(opts.Name) != 0 {
		sess = sess.Where("name=?", opts.Name)
	}

	sess = sess.Desc("id")

	if opts.Page != 0 {
		sess = setSessionPagination(sess, &opts)

		tokens := make([]*AccessToken, 0, opts.PageSize)
		return tokens, sess.Find(&tokens)
	}

	tokens := make([]*AccessToken, 0, 5)
	return tokens, sess.Find(&tokens)
}

// UpdateAccessToken updates information of access token.
func UpdateAccessToken(t *AccessToken) error {
	_, err := db.DefaultContext().Engine().ID(t.ID).AllCols().Update(t)
	return err
}

// CountAccessTokens count access tokens belongs to given user by options
func CountAccessTokens(opts ListAccessTokensOptions) (int64, error) {
	sess := db.DefaultContext().Engine().Where("uid=?", opts.UserID)
	if len(opts.Name) != 0 {
		sess = sess.Where("name=?", opts.Name)
	}
	return sess.Count(&AccessToken{})
}

// DeleteAccessTokenByID deletes access token by given ID.
func DeleteAccessTokenByID(id, userID int64) error {
	cnt, err := db.DefaultContext().Engine().ID(id).Delete(&AccessToken{
		UID: userID,
	})
	if err != nil {
		return err
	} else if cnt != 1 {
		return ErrAccessTokenNotExist{}
	}
	return nil
}
