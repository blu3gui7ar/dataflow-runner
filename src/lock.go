//
// Copyright (c) 2016-2017 Snowplow Analytics Ltd. All rights reserved.
//
// This program is licensed to you under the Apache License Version 2.0,
// and you may not use this file except in compliance with the Apache License Version 2.0.
// You may obtain a copy of the Apache License Version 2.0 at http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the Apache License Version 2.0 is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the Apache License Version 2.0 for the specific language governing permissions and limitations there under.
//

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"io/ioutil"

	"github.com/hashicorp/consul/api"
)

// Lock interface abstracting over file-based or consul-based locks
type Lock interface {
	TryLock() (bool, error)
	Unlock() error
}

// FileLock is for file-based locks
type FileLock struct {
	path string
}

// InitFileLock builds a FileLock at the path speicifed by name
func InitFileLock(name string) (Lock, error) {
	path, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	return &FileLock{path: path}, nil
}

// TryLock tries to acquire a lock on a file, returns true if the lock is already held
func (fl FileLock) TryLock() (bool, error) {
	// need to check that the file doesn't exist since we support locks surviving process shutdown
	if _, err := os.Stat(fl.path); err == nil {
		return true, nil
	}

	pid := os.Getppid()
	err := ioutil.WriteFile(fl.path, []byte(strconv.Itoa(pid)+"\n"), 0666)
	return false, err
}

// Unlock tries to release the lock on a file
func (fl FileLock) Unlock() error {
	return os.Remove(fl.path)
}

// ConsulLock is for Consul-based locks
type ConsulLock struct {
	kv  *api.KV
	key string
}

// InitConsulLock builds a ConsulLock (a KV pair in Consul) with the name argument as key
func InitConsulLock(consulAddress, name string) (Lock, error) {
	client, err := api.NewClient(&api.Config{Address: consulAddress})
	if err != nil {
		return nil, err
	}

	kv := client.KV()
	return &ConsulLock{kv: kv, key: name}, nil
}

// TryLock tries to acquire a lock from Consul
func (cl ConsulLock) TryLock() (bool, error) {
	p, _, err := cl.kv.Get(cl.key, nil)
	if err != nil {
		return false, err
	}
	if p != nil {
		return true, nil
	}
	pid := os.Getppid()
	_, err = cl.kv.Put(&api.KVPair{Key: cl.key, Value: []byte(strconv.Itoa(pid))}, nil)
	return false, err
}

// Unlock tries to release the lock from Consul
func (cl ConsulLock) Unlock() error {
	p, _, err := cl.kv.Get(cl.key, nil)
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("Lock not held")
	}
	_, err = cl.kv.Delete(cl.key, nil)
	return err
}

// GetLock builds a file-based or consul-based lock depending on the consul varialbe
func GetLock(lock, consul string) (Lock, error) {
	var l Lock
	var err error
	if consul != "" {
		l, err = InitConsulLock(consul, lock)
	} else {
		l, err = InitFileLock(lock)
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}