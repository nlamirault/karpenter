/*
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

package termination_test

import (
	"time"

	coretest "sigs.k8s.io/karpenter/pkg/test"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TerminationGracePeriod", func() {
	BeforeEach(func() {
		nodePool.Spec.Template.Spec.TerminationGracePeriod = &metav1.Duration{Duration: time.Second * 30}
	})
	It("should delete pod with do-not-disrupt when it reaches its terminationGracePeriodSeconds", func() {
		pod := coretest.UnschedulablePod(coretest.PodOptions{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			karpv1.DoNotDisruptAnnotationKey: "true",
		}}, TerminationGracePeriodSeconds: lo.ToPtr(int64(15))})
		env.ExpectCreated(nodeClass, nodePool, pod)

		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]
		env.EventuallyExpectHealthy(pod)

		// Delete the nodeclaim to start the TerminationGracePeriod
		env.ExpectDeleted(nodeClaim)

		// Eventually the node will be tainted
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).Should(Succeed())
			_, ok := lo.Find(node.Spec.Taints, func(t corev1.Taint) bool {
				return t.MatchTaint(&karpv1.DisruptedNoScheduleTaint)
			})
			g.Expect(ok).To(BeTrue())
		}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		// Check that pod remains healthy until termination grace period
		// subtract the polling time of the eventually above to reduce any races.
		env.ConsistentlyExpectHealthyPods(time.Second*15-100*time.Millisecond, pod)
		// Both nodeClaim and node should be gone once terminationGracePeriod is reached
		env.EventuallyExpectNotFound(nodeClaim, node, pod)
	})
	It("should delete pod that has a pre-stop hook after termination grace period seconds", func() {
		pod := coretest.UnschedulablePod(coretest.PodOptions{
			PreStopSleep:                  lo.ToPtr(int64(300)),
			TerminationGracePeriodSeconds: lo.ToPtr(int64(15)),
			Image:                         "alpine:3.20.2",
			Command:                       []string{"/bin/sh", "-c", "sleep 30"}})
		env.ExpectCreated(nodeClass, nodePool, pod)

		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]
		env.EventuallyExpectHealthy(pod)

		// Delete the nodeclaim to start the TerminationGracePeriod
		env.ExpectDeleted(nodeClaim)

		// Eventually the node will be tainted
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).Should(Succeed())
			_, ok := lo.Find(node.Spec.Taints, func(t corev1.Taint) bool {
				return t.MatchTaint(&karpv1.DisruptedNoScheduleTaint)
			})
			g.Expect(ok).To(BeTrue())
		}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		env.EventuallyExpectTerminating(pod)

		// Check that pod remains healthy until termination grace period
		// subtract the polling time of the eventually above to reduce any races.
		env.ConsistentlyExpectTerminatingPods(time.Second*15-100*time.Millisecond, pod)

		// Both nodeClaim and node should be gone once terminationGracePeriod is reached
		env.EventuallyExpectNotFound(nodeClaim, node, pod)
	})
})
