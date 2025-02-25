// Copyright 2017 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/auth/openid"
	"code.gitea.io/gitea/modules/log"
)

// ErrOpenIDNotExist openid is not known
var ErrOpenIDNotExist = errors.New("OpenID is unknown")

// UserOpenID is the list of all OpenID identities of a user.
type UserOpenID struct {
	ID   int64  `xorm:"pk autoincr"`
	UID  int64  `xorm:"INDEX NOT NULL"`
	URI  string `xorm:"UNIQUE NOT NULL"`
	Show bool   `xorm:"DEFAULT false"`
}

func init() {
	db.RegisterModel(new(UserOpenID))
}

// GetUserOpenIDs returns all openid addresses that belongs to given user.
func GetUserOpenIDs(uid int64) ([]*UserOpenID, error) {
	openids := make([]*UserOpenID, 0, 5)
	if err := db.DefaultContext().Engine().
		Where("uid=?", uid).
		Asc("id").
		Find(&openids); err != nil {
		return nil, err
	}

	return openids, nil
}

// isOpenIDUsed returns true if the openid has been used.
func isOpenIDUsed(e db.Engine, uri string) (bool, error) {
	if len(uri) == 0 {
		return true, nil
	}

	return e.Get(&UserOpenID{URI: uri})
}

// NOTE: make sure openid.URI is normalized already
func addUserOpenID(e db.Engine, openid *UserOpenID) error {
	used, err := isOpenIDUsed(e, openid.URI)
	if err != nil {
		return err
	} else if used {
		return ErrOpenIDAlreadyUsed{openid.URI}
	}

	_, err = e.Insert(openid)
	return err
}

// AddUserOpenID adds an pre-verified/normalized OpenID URI to given user.
func AddUserOpenID(openid *UserOpenID) error {
	return addUserOpenID(db.DefaultContext().Engine(), openid)
}

// DeleteUserOpenID deletes an openid address of given user.
func DeleteUserOpenID(openid *UserOpenID) (err error) {
	var deleted int64
	// ask to check UID
	address := UserOpenID{
		UID: openid.UID,
	}
	if openid.ID > 0 {
		deleted, err = db.DefaultContext().Engine().ID(openid.ID).Delete(&address)
	} else {
		deleted, err = db.DefaultContext().Engine().
			Where("openid=?", openid.URI).
			Delete(&address)
	}

	if err != nil {
		return err
	} else if deleted != 1 {
		return ErrOpenIDNotExist
	}
	return nil
}

// ToggleUserOpenIDVisibility toggles visibility of an openid address of given user.
func ToggleUserOpenIDVisibility(id int64) (err error) {
	_, err = db.DefaultContext().Engine().Exec("update `user_open_id` set `show` = not `show` where `id` = ?", id)
	return err
}

// GetUserByOpenID returns the user object by given OpenID if exists.
func GetUserByOpenID(uri string) (*User, error) {
	if len(uri) == 0 {
		return nil, ErrUserNotExist{0, uri, 0}
	}

	uri, err := openid.Normalize(uri)
	if err != nil {
		return nil, err
	}

	log.Trace("Normalized OpenID URI: " + uri)

	// Otherwise, check in openid table
	oid := &UserOpenID{}
	has, err := db.DefaultContext().Engine().Where("uri=?", uri).Get(oid)
	if err != nil {
		return nil, err
	}
	if has {
		return GetUserByID(oid.UID)
	}

	return nil, ErrUserNotExist{0, uri, 0}
}
