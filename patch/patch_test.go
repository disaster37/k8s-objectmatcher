// Copyright © 2021 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package patch

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	json "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCalculate(t *testing.T) {
	type args struct {
		modified *corev1.Service
		current  *corev1.Service
	}
	tests := []struct {
		name      string
		args      args
		wantPatch map[string]interface{}
		wantPatched *corev1.Service
		wantErr   bool
	}{
		{
			name: "non-existent field not deleted",
			args: args{
				modified: &corev1.Service{},
				current: &corev1.Service{},
			},
			wantPatch: map[string]any{},
			wantPatched: &corev1.Service{},
			wantErr: false,
		},
		{
			name: "add labels",
			args: args{
				modified: &corev1.Service{
					ObjectMeta: v1.ObjectMeta{
						Name: "my-service",
						Namespace: "default",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				current: &corev1.Service{
						ObjectMeta: v1.ObjectMeta{
							Name: "my-service",
							Namespace: "default",
						},
				},
			},
			wantPatch: map[string]any{
				"metadata": map[string]any {
					"labels": map[string]any {
						"foo": "bar",
					},
				},
			},
			wantPatched: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name: "my-service",
					Namespace: "default",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "remove labels",
			args: args{
				modified: &corev1.Service{
					ObjectMeta: v1.ObjectMeta{
						Name: "my-service",
						Namespace: "default",
					},
				},
				current: &corev1.Service{
					ObjectMeta: v1.ObjectMeta{
						Name: "my-service",
						Namespace: "default",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			wantPatch: map[string]any{
				"metadata": map[string]any {
					"labels": nil,
				},
			},
			wantPatched: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name: "my-service",
					Namespace: "default",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := DefaultPatchMaker.(*PatchMaker).Calculate(
				mustAnnotate(tt.args.current),
				tt.args.modified,
				CleanMetadata(),
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("Calculate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(mustToUnstructured(patch.Patch), tt.wantPatch); diff != "" {
				t.Errorf("Calculate() diff = %s,  got = %v, want %v", diff, mustToUnstructured(patch.Patch), tt.wantPatch)
			}
			if diff := cmp.Diff(patch.Patched, mustAnnotate(tt.wantPatched)); diff != "" {
				t.Errorf("Calculate() diff = %s,  got = %v, want %v", diff,  patch.Patched, mustAnnotate(tt.wantPatched))
			}
		})
	}
}

func Test_unstructuredJsonMergePatch(t *testing.T) {
	type args struct {
		original map[string]interface{}
		modified map[string]interface{}
		current  map[string]interface{}
	}
	tests := []struct {
		name      string
		args      args
		wantPatch map[string]interface{}
		wantPatched map[string]interface{}
		wantErr   bool
	}{
		{
			name: "non-existent field not deleted",
			args: args{
				original: map[string]interface{}{
					"a": "b",
				},
				modified: map[string]interface{}{},
				current:  map[string]interface{}{},
			},
			wantPatch: map[string]interface{}{},
			wantPatched: map[string]interface{}{},
			wantErr:   false,
		},
		{
			name: "existent field deleted",
			args: args{
				original: map[string]interface{}{
					"a": "b",
				},
				modified: map[string]interface{}{},
				current: map[string]interface{}{
					"a": "b",
				},
			},
			wantPatch: map[string]interface{}{
				"a": nil,
			},
			wantPatched: map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "existent field updated",
			args: args{
				modified: map[string]interface{}{
					"a": "new",
				},
				current: map[string]interface{}{
					"a": "b",
				},
			},
			wantPatch: map[string]interface{}{
				"a": "new",
			},
			wantPatched: map[string]interface{}{
				"a": "new",
			},
			wantErr: false,
		},
		{
			name: "new field added",
			args: args{
				modified: map[string]interface{}{
					"a": "b",
				},
				current: map[string]interface{}{},
			},
			wantPatch: map[string]interface{}{
				"a": "b",
			},
			wantPatched: map[string]interface{}{
				"a": "b",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, patched, err := DefaultPatchMaker.(*PatchMaker).unstructuredJsonMergePatch(
				mustFromUnstructured(tt.args.original),
				mustFromUnstructured(tt.args.modified),
				mustFromUnstructured(tt.args.current),
				mustFromUnstructured(tt.args.current))
			if (err != nil) != tt.wantErr {
				t.Errorf("unstructuredJsonMergePatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(mustToUnstructured(got), tt.wantPatch) {
				t.Errorf("unstructuredJsonMergePatch() got = %v, want %v", mustToUnstructured(got), tt.wantPatch)
			}
			if !reflect.DeepEqual(mustToUnstructured(patched), tt.wantPatched) {
				t.Errorf("unstructuredJsonMergePatch() got = %v, want %v", mustToUnstructured(patched), tt.wantPatched)
			}
		})
	}
}

func mustFromUnstructured(u map[string]interface{}) []byte {
	r, err := json.ConfigCompatibleWithStandardLibrary.Marshal(u)
	if err != nil {
		panic(err)
	}
	return r
}

func mustToUnstructured(data []byte) map[string]interface{} {
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		panic(err)
	}
	return m
}


func mustAnnotate(o runtime.Object) runtime.Object {
	if err := DefaultAnnotator.SetLastAppliedAnnotation(o); err != nil {
		panic(err)
	}
	return o
}