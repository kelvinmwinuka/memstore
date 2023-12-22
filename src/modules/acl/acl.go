package acl

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kelvinmwinuka/memstore/src/utils"
	"gopkg.in/yaml.v3"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"
)

type Password struct {
	PasswordType  string `json:"PasswordType" yaml:"PasswordType"` // plaintext, SHA256
	PasswordValue string `json:"PasswordValue" yaml:"PasswordValue"`
}

type Connection struct {
	Authenticated bool
	User          *User
}

type ACL struct {
	Users       []*User
	Connections map[*net.Conn]Connection
	Config      utils.Config
}

func NewACL(config utils.Config) *ACL {
	var users []*User

	// 1. Initialise default ACL user
	defaultUser := CreateUser("default")
	if config.RequirePass {
		defaultUser.NoPassword = false
		defaultUser.Passwords = []Password{
			{
				PasswordType:  GetPasswordType(config.Password),
				PasswordValue: config.Password,
			},
		}
	}

	// 2. Read and parse the ACL config file
	if config.AclConfig != "" {
		// Override acl configurations from file
		if f, err := os.Open(config.AclConfig); err != nil {
			panic(err)
		} else {
			defer func() {
				if err := f.Close(); err != nil {
					fmt.Println("acl config file close error: ", err)
				}
			}()

			ext := path.Ext(f.Name())

			if ext == ".json" {
				if err := json.NewDecoder(f).Decode(&users); err != nil {
					log.Fatal("could not load JSON ACL config: ", err)
				}
			}

			if ext == ".yaml" || ext == ".yml" {
				if err := yaml.NewDecoder(f).Decode(&users); err != nil {
					log.Fatal("could not load YAML ACL config: ", err)
				}
			}
		}
	}

	// 3. If default user was not loaded from file, add the created one
	defaultLoaded := false
	for _, user := range users {
		if user.Username == "default" {
			defaultLoaded = true
			break
		}
	}
	if !defaultLoaded {
		users = append([]*User{defaultUser}, users...)
	}

	// 4. Normalise all users
	for _, user := range users {
		user.Normalise()
	}

	acl := ACL{
		Users:       users,
		Connections: make(map[*net.Conn]Connection),
		Config:      config,
	}

	return &acl
}

func (acl *ACL) RegisterConnection(conn *net.Conn) {
	// This is called only when a connection is established.
	defaultUser := utils.Filter(acl.Users, func(elem *User) bool {
		return elem.Username == "default"
	})[0]
	acl.Connections[conn] = Connection{
		Authenticated: defaultUser.NoPassword,
		User:          defaultUser,
	}
}

func (acl *ACL) SetUser(ctx context.Context, cmd []string) error {
	// Check if user with the given username already exists
	// If it does, replace user variable with this user
	for _, user := range acl.Users {
		if user.Username == cmd[0] {
			return user.UpdateUser(cmd)
		}
	}

	user := CreateUser(cmd[0])
	if err := user.UpdateUser(cmd); err != nil {
		return err
	}

	user.Normalise()

	// Add user to ACL
	acl.Users = append(acl.Users, user)

	return nil
}

func (acl *ACL) DeleteUser(ctx context.Context, usernames []string) error {
	var user *User
	for _, username := range usernames {
		if username == "default" {
			// Skip default user
			continue
		}
		// Extract the user
		for _, u := range acl.Users {
			if username == u.Username {
				user = u
			}
		}
		// Skip if the current username was not found in the ACL
		if username != user.Username {
			continue
		}
		// Terminate every connection attached to this user
		for connRef, connection := range acl.Connections {
			if connection.User.Username == user.Username {
				(*connRef).SetReadDeadline(time.Now().Add(-1 * time.Second))
			}
		}
		// Delete the user from the ACL
		acl.Users = utils.Filter(acl.Users, func(u *User) bool {
			return u.Username != user.Username
		})
	}
	return nil
}

func (acl *ACL) AuthenticateConnection(ctx context.Context, conn *net.Conn, cmd []string) error {
	var passwords []Password
	var user *User

	h := sha256.New()

	if len(cmd) == 2 {
		// Process AUTH <password>
		h.Write([]byte(cmd[1]))
		passwords = []Password{
			{PasswordType: "plaintext", PasswordValue: cmd[1]},
			{PasswordType: "SHA256", PasswordValue: string(h.Sum(nil))},
		}
		// Authenticate with default user
		user = utils.Filter(acl.Users, func(user *User) bool {
			return user.Username == "default"
		})[0]
	}
	if len(cmd) == 3 {
		// Process AUTH <username> <password>
		h.Write([]byte(cmd[2]))
		passwords = []Password{
			{PasswordType: "plaintext", PasswordValue: cmd[2]},
			{PasswordType: "SHA256", PasswordValue: string(h.Sum(nil))},
		}
		// Find user with the specified username
		userFound := false
		for _, u := range acl.Users {
			if u.Username == cmd[1] {
				user = u
				userFound = true
				break
			}
		}
		if !userFound {
			return fmt.Errorf("no user with username %s", cmd[1])
		}
	}

	// If user is not enabled, return error
	if !user.Enabled {
		return fmt.Errorf("user %s is disabled", user.Username)
	}

	// If user is set to NoPassword, then immediately authenticate connection without considering the password
	if user.NoPassword {
		acl.Connections[conn] = Connection{
			Authenticated: true,
			User:          user,
		}
		return nil
	}

	for _, userPassword := range user.Passwords {
		for _, password := range passwords {
			if strings.EqualFold(userPassword.PasswordType, password.PasswordType) &&
				userPassword.PasswordValue == password.PasswordValue &&
				user.Enabled {
				// Set the current connection to the selected user and set them as authenticated
				acl.Connections[conn] = Connection{
					Authenticated: true,
					User:          user,
				}
				return nil
			}
		}
	}

	return errors.New("could not authenticate user")
}

func (acl *ACL) AuthorizeConnection(conn *net.Conn, cmd []string, command utils.Command, subCommand interface{}) error {
	// Extract command, categories, and keys
	comm := command.Command
	categories := command.Categories

	keys, err := command.KeyExtractionFunc(cmd)
	if err != nil {
		return err
	}

	if sub, ok := subCommand.(utils.SubCommand); ok {
		comm = fmt.Sprintf("%s|%s", comm, sub.Command)
		categories = append(categories, sub.Categories...)
		keys, err = sub.KeyExtractionFunc(cmd)
		if err != nil {
			return err
		}
	}

	// If the command is 'auth', then return early and allow it
	if strings.EqualFold(comm, "auth") {
		// TODO: Add rate limiting to prevent auth spamming
		return nil
	}

	// Get current connection ACL details
	connection := acl.Connections[conn]

	// 1. Check if password is required and if the user is authenticated
	if acl.Config.RequirePass && !connection.Authenticated {
		return errors.New("user must be authenticated")
	}

	// 2. PUBSUB authorisation comes first because it has slightly different handling.
	if utils.Contains(categories, utils.PubSubCategory) {
		// In PUBSUB, KeyExtractionFunc returns channels so "keys" is aliased to "channels" for clarity
		channels := keys
		// 2.1) Check if the channel is in IncludedPubSubChannels
		if !utils.Contains(connection.User.IncludedPubSubChannels, "*") {
			includedCount := make(map[string]int)
			for _, channel := range channels {
				includedCount[channel] = 0
			}
			for _, channel := range connection.User.IncludedPubSubChannels {
				includedCount[channel] += 1
			}
			for channel, count := range includedCount {
				if count == 0 {
					return fmt.Errorf("not authorised to access pubsub channel &%s", channel)
				}
			}
		}
		// 2.2) Check if the channel is in ExcludedPubSubChannels
		if utils.Contains(connection.User.ExcludedPubSubChannels, "*") {
			return errors.New("not authorised to access any pusub channels")
		} else {
			excludedChannels := make(map[string]int)
			for _, channel := range channels {
				excludedChannels[channel] = 0
			}
			for _, channel := range connection.User.ExcludedPubSubChannels {
				excludedChannels[channel] += 1
			}
			for channel, count := range excludedChannels {
				if count > 0 {
					return fmt.Errorf("not authorised to access pubsub channel &%s", channel)
				}
			}
		}
		return nil
	}

	// 3. Check if all categories are in IncludedCategories
	if !utils.Contains(connection.User.IncludedCategories, "*") {
		includedCount := make(map[string]int)
		for _, cat := range categories {
			includedCount[strings.ToLower(cat)] = 0
		}
		for _, cat := range connection.User.IncludedCategories {
			includedCount[strings.ToLower(cat)] += 1
		}
		for cat, count := range includedCount {
			if count == 0 {
				return fmt.Errorf("unauthorized to run @%s commands", cat)
			}
		}
	}

	// 4. Check if commands category is in ExcludedCategories
	if utils.Contains(connection.User.ExcludedCategories, "*") {
		return errors.New("not authorized to run @all commands")
	} else {
		excludedCount := make(map[string]int)
		for _, cat := range categories {
			excludedCount[strings.ToLower(cat)] = 0
		}
		for _, cat := range connection.User.ExcludedCategories {
			excludedCount[strings.ToLower(cat)] += 1
		}
		for cat, count := range excludedCount {
			if count > 0 {
				return fmt.Errorf("not authorized to run @%s commands", cat)
			}
		}
	}

	// 5. Check if commands are in IncludedCommands
	if !utils.Contains(connection.User.IncludedCommands, "*") {
		included := false
		for _, includedCommand := range connection.User.IncludedCommands {
			if strings.EqualFold(includedCommand, comm) {
				included = true
				break
			}
		}
		if !included {
			return fmt.Errorf("not authorised to run %s command", comm)
		}
	}

	// 6. Check if command are in ExcludedCommands
	if utils.Contains(connection.User.ExcludedCommands, "*") {
		return errors.New("not authorised to run any commands")
	} else {
		for _, excludedCommand := range connection.User.ExcludedCommands {
			if strings.EqualFold(excludedCommand, comm) {
				return fmt.Errorf("not authorised to run %s command", comm)
			}
		}
	}

	// 7. Check if keys are in IncludedKeys
	if !utils.Contains(connection.User.IncludedKeys, "*") {
		includedCount := make(map[string]int)
		for _, key := range keys {
			includedCount[key] = 0
		}
		for _, key := range connection.User.IncludedKeys {
			includedCount[key] += 1
		}
		for key, count := range includedCount {
			if count == 0 {
				return fmt.Errorf("not authorized to access key %s~%s", "%RW", key)
			}
		}
	}

	// 8. If @read is in the list of categories, check if keys are in IncludedReadKeys
	if utils.Contains(categories, utils.ReadCategory) && !utils.Contains(connection.User.IncludedReadKeys, "*") {
		includedCount := make(map[string]int)
		for _, key := range keys {
			includedCount[key] = 0
		}
		for _, key := range connection.User.IncludedReadKeys {
			includedCount[key] += 1
		}
		for key, count := range includedCount {
			if count == 0 {
				return fmt.Errorf("not authorised to acces key %s~%s", "%R", key)
			}
		}
	}

	// 9. If @write is in the list of categories, check if keys are in IncludedWriteKeys
	if utils.Contains(categories, utils.WriteCategory) && !utils.Contains(connection.User.IncludedWriteKeys, "*") {
		includedCount := make(map[string]int)
		for _, key := range keys {
			includedCount[key] = 0
		}
		for _, key := range connection.User.IncludedWriteKeys {
			includedCount[key] += 1
		}
		for key, count := range includedCount {
			if count == 0 {
				return fmt.Errorf("not authorised to acces key %s~%s", "%W", key)
			}
		}
	}

	return nil
}

func CreateUser(username string) *User {
	return &User{
		Username:               username,
		Enabled:                true,
		NoPassword:             false,
		Passwords:              []Password{},
		IncludedCategories:     []string{},
		ExcludedCategories:     []string{},
		IncludedCommands:       []string{},
		ExcludedCommands:       []string{},
		IncludedKeys:           []string{},
		IncludedReadKeys:       []string{},
		IncludedWriteKeys:      []string{},
		IncludedPubSubChannels: []string{},
		ExcludedPubSubChannels: []string{},
	}
}