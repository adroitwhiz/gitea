// Copyright 2016 The Gogs Authors. All rights reserved.
// Copyright 2020 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"net/mail"
	"strings"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"

	"xorm.io/builder"
)

// EmailAddress is the list of all email addresses of a user. It also contains the
// primary email address which is saved in user table.
type EmailAddress struct {
	ID          int64  `xorm:"pk autoincr"`
	UID         int64  `xorm:"INDEX NOT NULL"`
	Email       string `xorm:"UNIQUE NOT NULL"`
	LowerEmail  string `xorm:"UNIQUE NOT NULL"`
	IsActivated bool
	IsPrimary   bool `xorm:"DEFAULT(false) NOT NULL"`
}

func init() {
	db.RegisterModel(new(EmailAddress))
}

// BeforeInsert will be invoked by XORM before inserting a record
func (email *EmailAddress) BeforeInsert() {
	if email.LowerEmail == "" {
		email.LowerEmail = strings.ToLower(email.Email)
	}
}

// ValidateEmail check if email is a allowed address
func ValidateEmail(email string) error {
	if len(email) == 0 {
		return nil
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return ErrEmailInvalid{email}
	}

	// TODO: add an email allow/block list

	return nil
}

// GetEmailAddresses returns all email addresses belongs to given user.
func GetEmailAddresses(uid int64) ([]*EmailAddress, error) {
	emails := make([]*EmailAddress, 0, 5)
	if err := db.DefaultContext().Engine().
		Where("uid=?", uid).
		Asc("id").
		Find(&emails); err != nil {
		return nil, err
	}
	return emails, nil
}

// GetEmailAddressByID gets a user's email address by ID
func GetEmailAddressByID(uid, id int64) (*EmailAddress, error) {
	// User ID is required for security reasons
	email := &EmailAddress{UID: uid}
	if has, err := db.DefaultContext().Engine().ID(id).Get(email); err != nil {
		return nil, err
	} else if !has {
		return nil, nil
	}
	return email, nil
}

// isEmailActive check if email is activated with a different emailID
func isEmailActive(e db.Engine, email string, excludeEmailID int64) (bool, error) {
	if len(email) == 0 {
		return true, nil
	}

	// Can't filter by boolean field unless it's explicit
	cond := builder.NewCond()
	cond = cond.And(builder.Eq{"lower_email": strings.ToLower(email)}, builder.Neq{"id": excludeEmailID})
	if setting.Service.RegisterEmailConfirm {
		// Inactive (unvalidated) addresses don't count as active if email validation is required
		cond = cond.And(builder.Eq{"is_activated": true})
	}

	var em EmailAddress
	if has, err := e.Where(cond).Get(&em); has || err != nil {
		if has {
			log.Info("isEmailActive(%q, %d) found duplicate in email ID %d", email, excludeEmailID, em.ID)
		}
		return has, err
	}

	return false, nil
}

func isEmailUsed(e db.Engine, email string) (bool, error) {
	if len(email) == 0 {
		return true, nil
	}

	return e.Where("lower_email=?", strings.ToLower(email)).Get(&EmailAddress{})
}

// IsEmailUsed returns true if the email has been used.
func IsEmailUsed(email string) (bool, error) {
	return isEmailUsed(db.DefaultContext().Engine(), email)
}

func addEmailAddress(e db.Engine, email *EmailAddress) error {
	email.Email = strings.TrimSpace(email.Email)
	used, err := isEmailUsed(e, email.Email)
	if err != nil {
		return err
	} else if used {
		return ErrEmailAlreadyUsed{email.Email}
	}

	if err = ValidateEmail(email.Email); err != nil {
		return err
	}

	_, err = e.Insert(email)
	return err
}

// AddEmailAddress adds an email address to given user.
func AddEmailAddress(email *EmailAddress) error {
	return addEmailAddress(db.DefaultContext().Engine(), email)
}

// AddEmailAddresses adds an email address to given user.
func AddEmailAddresses(emails []*EmailAddress) error {
	if len(emails) == 0 {
		return nil
	}

	// Check if any of them has been used
	for i := range emails {
		emails[i].Email = strings.TrimSpace(emails[i].Email)
		used, err := IsEmailUsed(emails[i].Email)
		if err != nil {
			return err
		} else if used {
			return ErrEmailAlreadyUsed{emails[i].Email}
		}
		if err = ValidateEmail(emails[i].Email); err != nil {
			return err
		}
	}

	if _, err := db.DefaultContext().Engine().Insert(emails); err != nil {
		return fmt.Errorf("Insert: %v", err)
	}

	return nil
}

// Activate activates the email address to given user.
func (email *EmailAddress) Activate() error {
	sess := db.DefaultContext().NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	if err := email.updateActivation(sess, true); err != nil {
		return err
	}
	return sess.Commit()
}

func (email *EmailAddress) updateActivation(e db.Engine, activate bool) error {
	user, err := getUserByID(e, email.UID)
	if err != nil {
		return err
	}
	if user.Rands, err = GetUserSalt(); err != nil {
		return err
	}
	email.IsActivated = activate
	if _, err := e.ID(email.ID).Cols("is_activated").Update(email); err != nil {
		return err
	}
	return updateUserCols(e, user, "rands")
}

// DeleteEmailAddress deletes an email address of given user.
func DeleteEmailAddress(email *EmailAddress) (err error) {
	if email.IsPrimary {
		return ErrPrimaryEmailCannotDelete{Email: email.Email}
	}

	var deleted int64
	// ask to check UID
	address := EmailAddress{
		UID: email.UID,
	}
	if email.ID > 0 {
		deleted, err = db.DefaultContext().Engine().ID(email.ID).Delete(&address)
	} else {
		if email.Email != "" && email.LowerEmail == "" {
			email.LowerEmail = strings.ToLower(email.Email)
		}
		deleted, err = db.DefaultContext().Engine().
			Where("lower_email=?", email.LowerEmail).
			Delete(&address)
	}

	if err != nil {
		return err
	} else if deleted != 1 {
		return ErrEmailAddressNotExist{Email: email.Email}
	}
	return nil
}

// DeleteEmailAddresses deletes multiple email addresses
func DeleteEmailAddresses(emails []*EmailAddress) (err error) {
	for i := range emails {
		if err = DeleteEmailAddress(emails[i]); err != nil {
			return err
		}
	}

	return nil
}

// MakeEmailPrimary sets primary email address of given user.
func MakeEmailPrimary(email *EmailAddress) error {
	has, err := db.DefaultContext().Engine().Get(email)
	if err != nil {
		return err
	} else if !has {
		return ErrEmailAddressNotExist{Email: email.Email}
	}

	if !email.IsActivated {
		return ErrEmailNotActivated
	}

	user := &User{}
	has, err = db.DefaultContext().Engine().ID(email.UID).Get(user)
	if err != nil {
		return err
	} else if !has {
		return ErrUserNotExist{email.UID, "", 0}
	}

	sess := db.DefaultContext().NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	// 1. Update user table
	user.Email = email.Email
	if _, err = sess.ID(user.ID).Cols("email").Update(user); err != nil {
		return err
	}

	// 2. Update old primary email
	if _, err = sess.Where("uid=? AND is_primary=?", email.UID, true).Cols("is_primary").Update(&EmailAddress{
		IsPrimary: false,
	}); err != nil {
		return err
	}

	// 3. update new primary email
	email.IsPrimary = true
	if _, err = sess.ID(email.ID).Cols("is_primary").Update(email); err != nil {
		return err
	}

	return sess.Commit()
}

// SearchEmailOrderBy is used to sort the results from SearchEmails()
type SearchEmailOrderBy string

func (s SearchEmailOrderBy) String() string {
	return string(s)
}

// Strings for sorting result
const (
	SearchEmailOrderByEmail        SearchEmailOrderBy = "email_address.lower_email ASC, email_address.is_primary DESC, email_address.id ASC"
	SearchEmailOrderByEmailReverse SearchEmailOrderBy = "email_address.lower_email DESC, email_address.is_primary ASC, email_address.id DESC"
	SearchEmailOrderByName         SearchEmailOrderBy = "`user`.lower_name ASC, email_address.is_primary DESC, email_address.id ASC"
	SearchEmailOrderByNameReverse  SearchEmailOrderBy = "`user`.lower_name DESC, email_address.is_primary ASC, email_address.id DESC"
)

// SearchEmailOptions are options to search e-mail addresses for the admin panel
type SearchEmailOptions struct {
	ListOptions
	Keyword     string
	SortType    SearchEmailOrderBy
	IsPrimary   util.OptionalBool
	IsActivated util.OptionalBool
}

// SearchEmailResult is an e-mail address found in the user or email_address table
type SearchEmailResult struct {
	UID         int64
	Email       string
	IsActivated bool
	IsPrimary   bool
	// From User
	Name     string
	FullName string
}

// SearchEmails takes options i.e. keyword and part of email name to search,
// it returns results in given range and number of total results.
func SearchEmails(opts *SearchEmailOptions) ([]*SearchEmailResult, int64, error) {
	var cond builder.Cond = builder.Eq{"`user`.`type`": UserTypeIndividual}
	if len(opts.Keyword) > 0 {
		likeStr := "%" + strings.ToLower(opts.Keyword) + "%"
		cond = cond.And(builder.Or(
			builder.Like{"lower(`user`.full_name)", likeStr},
			builder.Like{"`user`.lower_name", likeStr},
			builder.Like{"email_address.lower_email", likeStr},
		))
	}

	switch {
	case opts.IsPrimary.IsTrue():
		cond = cond.And(builder.Eq{"email_address.is_primary": true})
	case opts.IsPrimary.IsFalse():
		cond = cond.And(builder.Eq{"email_address.is_primary": false})
	}

	switch {
	case opts.IsActivated.IsTrue():
		cond = cond.And(builder.Eq{"email_address.is_activated": true})
	case opts.IsActivated.IsFalse():
		cond = cond.And(builder.Eq{"email_address.is_activated": false})
	}

	count, err := db.DefaultContext().Engine().Join("INNER", "`user`", "`user`.ID = email_address.uid").
		Where(cond).Count(new(EmailAddress))
	if err != nil {
		return nil, 0, fmt.Errorf("Count: %v", err)
	}

	orderby := opts.SortType.String()
	if orderby == "" {
		orderby = SearchEmailOrderByEmail.String()
	}

	opts.setDefaultValues()

	emails := make([]*SearchEmailResult, 0, opts.PageSize)
	err = db.DefaultContext().Engine().Table("email_address").
		Select("email_address.*, `user`.name, `user`.full_name").
		Join("INNER", "`user`", "`user`.ID = email_address.uid").
		Where(cond).
		OrderBy(orderby).
		Limit(opts.PageSize, (opts.Page-1)*opts.PageSize).
		Find(&emails)

	return emails, count, err
}

// ActivateUserEmail will change the activated state of an email address,
// either primary or secondary (all in the email_address table)
func ActivateUserEmail(userID int64, email string, activate bool) (err error) {
	sess := db.DefaultContext().NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	// Activate/deactivate a user's secondary email address
	// First check if there's another user active with the same address
	addr := EmailAddress{UID: userID, LowerEmail: strings.ToLower(email)}
	if has, err := sess.Get(&addr); err != nil {
		return err
	} else if !has {
		return fmt.Errorf("no such email: %d (%s)", userID, email)
	}
	if addr.IsActivated == activate {
		// Already in the desired state; no action
		return nil
	}
	if activate {
		if used, err := isEmailActive(sess, email, addr.ID); err != nil {
			return fmt.Errorf("unable to check isEmailActive() for %s: %v", email, err)
		} else if used {
			return ErrEmailAlreadyUsed{Email: email}
		}
	}
	if err = addr.updateActivation(sess, activate); err != nil {
		return fmt.Errorf("unable to updateActivation() for %d:%s: %w", addr.ID, addr.Email, err)
	}

	// Activate/deactivate a user's primary email address and account
	if addr.IsPrimary {
		user := User{ID: userID, Email: email}
		if has, err := sess.Get(&user); err != nil {
			return err
		} else if !has {
			return fmt.Errorf("no user with ID: %d and Email: %s", userID, email)
		}
		// The user's activation state should be synchronized with the primary email
		if user.IsActive != activate {
			user.IsActive = activate
			if user.Rands, err = GetUserSalt(); err != nil {
				return fmt.Errorf("unable to generate salt: %v", err)
			}
			if err = updateUserCols(sess, &user, "is_active", "rands"); err != nil {
				return fmt.Errorf("unable to updateUserCols() for user ID: %d: %v", userID, err)
			}
		}
	}

	return sess.Commit()
}
