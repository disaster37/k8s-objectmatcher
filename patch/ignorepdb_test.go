package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	policyv1 "k8s.io/api/policy/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIgnorePDBSelector(t *testing.T) {
	var (
		currentPdb  *policyv1.PodDisruptionBudget
		expectedPdb *policyv1.PodDisruptionBudget
		err         error
		patch       *PatchResult
	)

	// Same PDB
	currentPdb = &policyv1.PodDisruptionBudget{
		TypeMeta: v1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "pdb",
			Namespace: "default",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 1},
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
					"env": "test",
				},
			},
		},
	}
	expectedPdb = &policyv1.PodDisruptionBudget{
		TypeMeta: v1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "pdb",
			Namespace: "default",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 1},
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
					"env": "test",
				},
			},
		},
	}

	// Proof is not match without ignore IgnorePDBSelector
	patch, err = DefaultPatchMaker.Calculate(currentPdb, expectedPdb, CleanMetadata())
	assert.NoError(t, err)
	assert.False(t, patch.IsEmpty())

	// Proof is match with IgnorePDBSelector
	patch, err = DefaultPatchMaker.Calculate(currentPdb, expectedPdb, CleanMetadata(), IgnorePDBSelector())
	assert.NoError(t, err)
	assert.True(t, patch.IsEmpty())

	// Proof it detect diff
	expectedPdb = &policyv1.PodDisruptionBudget{
		TypeMeta: v1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "pdb",
			Namespace: "default",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 1},
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "other",
					"env": "test",
				},
			},
		},
	}

	patch, err = DefaultPatchMaker.Calculate(currentPdb, expectedPdb, CleanMetadata(), IgnorePDBSelector())
	assert.NoError(t, err)
	assert.False(t, patch.IsEmpty())

}
