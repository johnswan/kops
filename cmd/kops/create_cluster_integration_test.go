/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"io/ioutil"
	"k8s.io/kops/cloudmock/aws/mockec2"
	"k8s.io/kops/cloudmock/aws/mockroute53"
	"k8s.io/kops/cmd/kops/util"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/diff"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/util/pkg/vfs"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"path"
	"strings"
	"testing"
	"time"
)

var MagicTimestamp = unversioned.Time{Time: time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)}

// TestMinimal runs kops create cluster minimal.example.com --zones us-test-1a
func TestCreateClusterMinimal(t *testing.T) {
	runCreateClusterIntegrationTest(t, "../../tests/integration/create_cluster/minimal", 2)
}

func runCreateClusterIntegrationTest(t *testing.T, srcDir string, expectedInstanceGroups int) {
	var stdout bytes.Buffer

	optionsYAML := "options.yaml"
	expectedClusterPath := "cluster.yaml"

	factoryOptions := &util.FactoryOptions{}
	factoryOptions.RegistryPath = "memfs://tests"

	vfs.Context.ResetMemfsContext(true)

	cloud := awsup.InstallMockAWSCloud("us-test-1", "abc")
	mockEC2 := &mockec2.MockEC2{}
	cloud.MockEC2 = mockEC2
	mockRoute53 := &mockroute53.MockRoute53{}
	cloud.MockRoute53 = mockRoute53

	mockRoute53.Zones = append(mockRoute53.Zones, &route53.HostedZone{
		Id:   aws.String("/hostedzone/Z1AFAKE1ZON3YO"),
		Name: aws.String("example.com."),
	})

	factory := util.NewFactory(factoryOptions)

	{
		optionsBytes, err := ioutil.ReadFile(path.Join(srcDir, optionsYAML))
		if err != nil {
			t.Fatalf("error reading options file: %v", err)
		}

		options := &CreateClusterOptions{}
		options.InitDefaults()

		err = kops.ParseRawYaml(optionsBytes, options)
		if err != nil {
			t.Fatalf("error parsing options: %v", err)
		}

		// No preview
		options.Target = ""

		err = RunCreateCluster(factory, &stdout, options)
		if err != nil {
			t.Fatalf("error running create cluster: %v", err)
		}
	}

	clientset, err := factory.Clientset()
	if err != nil {
		t.Fatalf("error getting clientset: %v", err)
	}

	// Compare cluster
	clusters, err := clientset.Clusters().List(k8sapi.ListOptions{})
	if err != nil {
		t.Fatalf("error listing clusters: %v", err)
	}

	if len(clusters.Items) != 1 {
		t.Fatalf("expected one cluster, found %d", len(clusters.Items))
	}
	for _, cluster := range clusters.Items {
		cluster.ObjectMeta.CreationTimestamp = MagicTimestamp
		actualYAMLBytes, err := kops.ToVersionedYaml(&cluster)
		if err != nil {
			t.Fatalf("unexpected error serializing cluster: %v", err)
		}
		expectedYAMLBytes, err := ioutil.ReadFile(path.Join(srcDir, expectedClusterPath))
		if err != nil {
			t.Fatalf("unexpected error reading expected cluster: %v", err)
		}

		actualYAML := strings.TrimSpace(string(actualYAMLBytes))
		expectedYAML := strings.TrimSpace(string(expectedYAMLBytes))

		if actualYAML != expectedYAML {
			glog.Infof("Actual cluster:\n%s\n", actualYAML)

			diffString := diff.FormatDiff(expectedYAML, actualYAML)
			t.Logf("diff:\n%s\n", diffString)

			t.Fatalf("cluster differed from expected")
		}
	}

	// Compare instance groups

	instanceGroups, err := clientset.InstanceGroups(clusters.Items[0].ObjectMeta.Name).List(k8sapi.ListOptions{})
	if err != nil {
		t.Fatalf("error listing instance groups: %v", err)
	}

	if len(instanceGroups.Items) != expectedInstanceGroups {
		t.Fatalf("expected %d instance groups, found %d", expectedInstanceGroups, len(instanceGroups.Items))
	}
	for _, ig := range instanceGroups.Items {
		ig.ObjectMeta.CreationTimestamp = MagicTimestamp

		actualYAMLBytes, err := kops.ToVersionedYaml(&ig)
		if err != nil {
			t.Fatalf("unexpected error serializing InstanceGroup: %v", err)
		}
		expectedYAMLBytes, err := ioutil.ReadFile(path.Join(srcDir, ig.ObjectMeta.Name+".yaml"))
		if err != nil {
			t.Fatalf("unexpected error reading expected InstanceGroup: %v", err)
		}

		actualYAML := strings.TrimSpace(string(actualYAMLBytes))
		expectedYAML := strings.TrimSpace(string(expectedYAMLBytes))

		if actualYAML != expectedYAML {
			glog.Infof("Actual IG %q:\n%s\n", ig.ObjectMeta.Name, actualYAML)

			diffString := diff.FormatDiff(expectedYAML, actualYAML)
			t.Logf("diff:\n%s\n", diffString)

			t.Fatalf("instance group %q differed from expected", ig.ObjectMeta.Name)
		}
	}
}
