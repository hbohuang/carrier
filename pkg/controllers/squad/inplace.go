// Copyright 2020 THL A29 Limited, a Tencent company.
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

package squad

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"

	carrierv1alpha1 "github.com/ocgi/carrier/pkg/apis/carrier/v1alpha1"
	"github.com/ocgi/carrier/pkg/util"
	"github.com/ocgi/carrier/pkg/util/kube"
)

// inplace update gameserver inplace
func (c *Controller) rolloutInplace(squad *carrierv1alpha1.Squad, gsSetList []*carrierv1alpha1.GameServerSet) error {
	newGSSet, isFirstCreate, err := c.findOrCreateGameServerSet(squad, gsSetList)
	if err != nil {
		return err
	}

	var allGSSet []*carrierv1alpha1.GameServerSet
	allGSSet = append(allGSSet, gsSetList...)

	threshold := InplaceThreshold(*squad)
	if threshold == 0 || isFirstCreate {
		// Do nothing if the threshold is zero
		// or the first creation
		if isFirstCreate {
			if err := c.clearInplaceUpdateStrategy(squad); err != nil {
				klog.Errorf("clear threshold failed: %v", err)
				return err
			}
			allGSSet = append(allGSSet, newGSSet)
		}
		return c.syncRolloutStatus(allGSSet, newGSSet, squad)
	}
	// update gameserver set
	SetGameServerSetInplaceUpdateAnnotations(newGSSet, squad)
	SetGameServerTemplateHashLabels(newGSSet)
	newGSSet.Spec.Template.Spec.Template.Spec = *squad.Spec.Template.Spec.Template.Spec.DeepCopy()
	_, err = c.gameServerSetGetter.GameServerSets(newGSSet.Namespace).Update(newGSSet)
	if err != nil {
		return err
	}
	if threshold < squad.Spec.Replicas {
		return c.syncRolloutStatus(allGSSet, newGSSet, squad)
	}
	if SquadComplete(squad, &squad.Status) {
		if err := c.cleanupGameServerSet(newGSSet); err != nil {
			return err
		}
		if err := c.clearInplaceUpdateStrategy(squad); err != nil {
			return err
		}
	}
	// Sync Squad status
	return c.syncRolloutStatus(allGSSet, newGSSet, squad)
}

func (c *Controller) cleanupGameServerSet(gsSet *carrierv1alpha1.GameServerSet) error {
	klog.V(4).Infof("Cleans up inplace update annotations of gameserver set %q", gsSet.Name)
	delete(gsSet.Annotations, util.GameServerInPlaceUpdateAnnotation)
	delete(gsSet.Annotations, util.GameServerInPlaceUpdatedReplicasAnnotation)
	_, err := c.gameServerSetGetter.GameServerSets(gsSet.Namespace).Update(gsSet)
	return err
}

// clearInplaceUpdateThreshold sets .spec.strategy.inplaceUpdate.threshold to zero and update the input Squad
func (c *Controller) clearInplaceUpdateStrategy(squad *carrierv1alpha1.Squad) error {
	klog.V(4).Infof("Cleans up threshold (%v) of squad %q", squad.Spec.Strategy.InplaceUpdate, squad.Name)
	squadCopy := squad.DeepCopy()
	threshold := intstr.FromInt(0)
	squadCopy.Spec.Strategy.InplaceUpdate.Threshold = &threshold
	patch, err := kube.CreateMergePatch(squad, squadCopy)
	if err != nil {
		return err
	}
	_, err = c.squadGetter.Squads(squad.Namespace).Patch(squad.Name, types.MergePatchType, patch)
	return err
}
