// Copyright © 2019 Banzai Cloud
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
	"fmt"
	"reflect"

	"emperror.dev/errors"
	json "github.com/json-iterator/go"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var DefaultPatchMaker = NewPatchMaker(DefaultAnnotator, &K8sStrategicMergePatcher{}, &BaseJSONMergePatcher{})

type Maker interface {
	Calculate(currentObject, modifiedObject runtime.Object, opts ...CalculateOption) (*PatchResult, error)
}

type PatchMaker struct {
	annotator *Annotator

	strategicMergePatcher StrategicMergePatcher
	jsonMergePatcher      JSONMergePatcher
}

func NewPatchMaker(annotator *Annotator, strategicMergePatcher StrategicMergePatcher, jsonMergePatcher JSONMergePatcher) Maker {
	return &PatchMaker{
		annotator: annotator,

		strategicMergePatcher: strategicMergePatcher,
		jsonMergePatcher:      jsonMergePatcher,
	}
}

func (p *PatchMaker) Calculate(currentObject, modifiedObject runtime.Object, opts ...CalculateOption) (*PatchResult, error) {

	current, err := json.ConfigCompatibleWithStandardLibrary.Marshal(currentObject)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to convert current object to byte sequence")
	}
	currentOrg := make([]byte, len(current))
	copy(currentOrg, current)

	modified, err := json.ConfigCompatibleWithStandardLibrary.Marshal(modifiedObject)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to convert current object to byte sequence")
	}

	for _, opt := range opts {
		current, modified, err = opt(current, modified)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to apply option function")
		}
	}

	current, _, err = DeleteNullInJson(current)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to delete null from current object")
	}

	modified, _, err = DeleteNullInJson(modified)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to delete null from modified object")
	}

	original, err := p.annotator.GetOriginalConfiguration(currentObject)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get original configuration")
	}

	var patch []byte
	var patched any
	var patchedCurrent []byte

	switch currentObject.(type) {
	default:
		patch, err = p.strategicMergePatcher.CreateThreeWayMergePatch(original, modified, current, currentObject)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to generate strategic merge patch")
		}

		// $setElementOrder can make it hard to decide whether there is an actual diff or not.
		// In cases like that trying to apply the patch locally on current will make it clear.
		if string(patch) != "{}" {
			patchCurrent, err := p.strategicMergePatcher.StrategicMergePatch(current, patch, currentObject)
			if err != nil {
				return nil, errors.Wrap(err, "Failed to apply patch")
			}

			patch, err = p.strategicMergePatcher.CreateTwoWayMergePatch(current, patchCurrent, currentObject)
			if err != nil {
				return nil, errors.Wrap(err, "Failed to create patch again to check for an actual diff")
			}

			patchedCurrent, err = p.strategicMergePatcher.StrategicMergePatch(currentOrg, patch, currentObject)
			if err != nil {
				return nil, errors.Wrap(err, "Failed to apply patch")
			}
		} else {
			patchedCurrent = currentOrg
		}

		switch reflect.ValueOf(currentObject).Kind() {
		case reflect.Ptr:
			patched = reflect.New(reflect.ValueOf(currentObject).Elem().Type()).Interface()
			if err = json.Unmarshal(patchedCurrent, patched); err != nil {
				return nil, errors.Wrap(err, "Failed to create patched object")
			}
		case reflect.Struct:
			patched = reflect.New(reflect.ValueOf(currentObject).Type()).Interface()
			if err = json.Unmarshal(patchedCurrent, patched); err != nil {
				return nil, errors.Wrap(err, "Failed to create patched object")
			}
		default:
			panic(fmt.Sprintf("Unknow type: %s", reflect.ValueOf(currentObject).Kind()))
		}
		if err := DefaultAnnotator.SetLastAppliedAnnotationToObject(patched.(runtime.Object), modifiedObject); err != nil {
			return nil, errors.Wrap(err, "Failed to annotate patched object")
		}
	case *unstructured.Unstructured:
		var patchCurrent []byte
		patch, patchCurrent, err = p.unstructuredJsonMergePatch(original, modified, current, currentOrg)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to generate merge patch")
		}

		patched = reflect.New(reflect.ValueOf(currentObject).Elem().Type()).Interface()
		if err = json.Unmarshal(patchCurrent, patched); err != nil {
			return nil, errors.Wrap(err, "Failed to create patched object")
		}

		if err := DefaultAnnotator.SetLastAppliedAnnotationToObject(patched.(runtime.Object), modifiedObject); err != nil {
			return nil, errors.Wrap(err, "Failed to annotate patched object")
		}
	}

	return &PatchResult{
		Patch:    patch,
		Current:  current,
		Modified: modified,
		Original: original,
		Patched:  patched,
	}, nil
}

func (p *PatchMaker) unstructuredJsonMergePatch(original, modified, current, currentOrg []byte) ([]byte, []byte, error) {
	
	patch, err := p.jsonMergePatcher.CreateThreeWayJSONMergePatch(original, modified, current)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to generate merge patch")
	}

	var patchedCurrent []byte

	// Apply the patch to the current object and create a merge patch to see if there has any effective changes been made
	if string(patch) != "{}" {
		// apply the patch
		patchCurrent, err := p.jsonMergePatcher.MergePatch(current, patch)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to merge generated patch to current object")
		}
		// create the patch again, but now between the current and the patched version of the current object
		patch, err = p.jsonMergePatcher.CreateMergePatch(current, patchCurrent)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to create patch between the current and patched current object")
		}

		patchedCurrent, err = p.jsonMergePatcher.MergePatch(currentOrg, patch)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to apply patch")
		}
	} else {
		patchedCurrent = currentOrg
	}
	return patch, patchedCurrent, err
}

type PatchResult struct {
	Patch    []byte
	Current  []byte
	Modified []byte
	Original []byte
	Patched  any
}

func (p *PatchResult) IsEmpty() bool {
	return string(p.Patch) == "{}"
}

func (p *PatchResult) String() string {
	return fmt.Sprintf("\nPatch: %s \nCurrent: %s\nModified: %s\nOriginal: %s\n", p.Patch, p.Current, p.Modified, p.Original)
}
