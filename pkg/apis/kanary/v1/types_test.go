// Copyright (c) 2017 Chef Software Inc. and/or applicable contributors
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

package v1

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/google/gofuzz"

	"k8s.io/apimachinery/pkg/api/testing/fuzzer"
	roundtrip "k8s.io/apimachinery/pkg/api/testing/roundtrip"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

var _ runtime.Object = &Kanary{}
var _ metav1.ObjectMetaAccessor = &Kanary{}

var _ runtime.Object = &KanaryList{}
var _ metav1.ListMetaAccessor = &KanaryList{}

func habitatFuzzerFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(obj *KanaryList, c fuzz.Continue) {
			c.FuzzNoCustom(obj)
			obj.Items = make([]Kanary, c.Intn(10))
			for i := range obj.Items {
				c.Fuzz(&obj.Items[i])
			}
		},
	}
}

// TestRoundTrip tests that the third-party kinds can be marshaled and unmarshaled correctly to/from JSON
// without the loss of information. Moreover, deep copy is tested.
func TestRoundTrip(t *testing.T) {
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	AddToScheme(scheme)

	seed := rand.Int63()
	fuzzerFuncs := fuzzer.MergeFuzzerFuncs(metafuzzer.Funcs, habitatFuzzerFuncs)
	fuzzer := fuzzer.FuzzerFor(fuzzerFuncs, rand.NewSource(seed), codecs)

	roundtrip.RoundTripSpecificKindWithoutProtobuf(t, SchemeGroupVersion.WithKind("Kanary"), scheme, codecs, fuzzer, nil)
	roundtrip.RoundTripSpecificKindWithoutProtobuf(t, SchemeGroupVersion.WithKind("KanaryList"), scheme, codecs, fuzzer, nil)
}

func TestHAProxyCreationNeeded(t *testing.T) {
	testCases := []struct {
		testName string
		ky       Kanary
		expected bool
	}{
		{
			testName: "simple absent",
			ky: Kanary{
				Status: KanaryStatus{
					DestinationStatus: DestinationStatus{
						ProxyName:   "",
						ServiceName: "",
						ConfigName:  "",
					},
				},
			},
			expected: true,
		},
		{
			testName: "simple present",
			ky: Kanary{
				Status: KanaryStatus{
					DestinationStatus: DestinationStatus{
						ProxyName:   "haproxyname",
						ServiceName: "",
						ConfigName:  "",
					},
				},
			},
			expected: false,
		},
	}

	for _, test := range testCases {
		result := test.ky.HAProxyCreationNeeded()
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("FAIL %s, Return func: %v should be equal to expected %v", test.testName, result, test.expected)
		}
	}
}
