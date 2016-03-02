/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package replicaset

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/apis/extensions"
	unversionedextensions "k8s.io/kubernetes/pkg/client/typed/generated/extensions/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
)

// TODO: use client library instead when it starts to support update retries
//       see https://github.com/kubernetes/kubernetes/issues/21479
type updateRSFunc func(rs *extensions.ReplicaSet)
type preconditionFunc func(rs *extensions.ReplicaSet) bool

// UpdateRSWithRetries updates a RS with given applyUpdate function, when the given precondition holds. Note that RS not found error is ignored.
// The returned bool value can be used to tell if the RS is actually updated.
func UpdateRSWithRetries(rsClient unversionedextensions.ReplicaSetInterface, rs *extensions.ReplicaSet, preconditionHold preconditionFunc, applyUpdate updateRSFunc) (*extensions.ReplicaSet, bool, error) {
	var err error
	var rsUpdated bool
	oldRs := rs
	if err = wait.Poll(10*time.Millisecond, 1*time.Minute, func() (bool, error) {
		rs, err = rsClient.Get(oldRs.Name)
		if err != nil {
			return false, err
		}
		if !preconditionHold(rs) {
			glog.V(4).Infof("rs %s precondition doesn't hold, skip updating it.", rs.Name)
			return true, nil
		}
		// Apply the update, then attempt to push it to the apiserver.
		applyUpdate(rs)
		if rs, err = rsClient.Update(rs); err == nil {
			// Update successful.
			return true, nil
		}
		// TODO: don't retry on perm-failed errors and handle them gracefully
		// Update could have failed due to conflict error. Try again.
		return false, nil
	}); err == nil {
		// When there's no error, we've updated this RS.
		rsUpdated = true
	}

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("timed out trying to update RS: %+v", oldRs)
	}
	// Ignore the RS not found error, but the RS isn't updated.
	if errors.IsNotFound(err) {
		glog.V(4).Infof("%s %s/%s is not found, skip updating it.", oldRs.Kind, oldRs.Namespace, oldRs.Name)
		err = nil
	}
	// If the error is non-nil the returned controller cannot be trusted, if it is nil, the returned
	// controller contains the applied update.
	return rs, rsUpdated, err
}
