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

package cloudup

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kopsbase "k8s.io/kops"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/kops/registry"
	"k8s.io/kops/pkg/apis/kops/util"
	"k8s.io/kops/pkg/apis/kops/validation"
	"k8s.io/kops/pkg/apis/nodeup"
	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/pkg/dns"
	"k8s.io/kops/pkg/featureflag"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/pkg/model/awsmodel"
	"k8s.io/kops/pkg/model/components"
	"k8s.io/kops/pkg/model/domodel"
	"k8s.io/kops/pkg/model/gcemodel"
	"k8s.io/kops/pkg/model/openstackmodel"
	"k8s.io/kops/pkg/model/vspheremodel"
	"k8s.io/kops/pkg/resources/digitalocean"
	"k8s.io/kops/pkg/templates"
	"k8s.io/kops/upup/models"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awstasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/baremetal"
	"k8s.io/kops/upup/pkg/fi/cloudup/cloudformation"
	"k8s.io/kops/upup/pkg/fi/cloudup/do"
	"k8s.io/kops/upup/pkg/fi/cloudup/dotasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
	"k8s.io/kops/upup/pkg/fi/cloudup/gcetasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstack"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstacktasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
	"k8s.io/kops/upup/pkg/fi/cloudup/vsphere"
	"k8s.io/kops/upup/pkg/fi/cloudup/vspheretasks"
	"k8s.io/kops/upup/pkg/fi/fitasks"
	"k8s.io/kops/util/pkg/vfs"
)

const (
	DefaultMaxTaskDuration = 10 * time.Minute
	starline               = "*********************************************************************************\n"
)

var (
	// AlphaAllowBareMetal is a feature flag that gates BareMetal support while it is alpha
	AlphaAllowBareMetal = featureflag.New("AlphaAllowBareMetal", featureflag.Bool(false))
	// AlphaAllowDO is a feature flag that gates DigitalOcean support while it is alpha
	AlphaAllowDO = featureflag.New("AlphaAllowDO", featureflag.Bool(false))
	// AlphaAllowGCE is a feature flag that gates GCE support while it is alpha
	AlphaAllowGCE = featureflag.New("AlphaAllowGCE", featureflag.Bool(false))
	// AlphaAllowVsphere is a feature flag that gates vsphere support while it is alpha
	AlphaAllowVsphere = featureflag.New("AlphaAllowVsphere", featureflag.Bool(false))
	// CloudupModels a list of supported models
	CloudupModels = []string{"config", "proto", "cloudup"}
)

type ApplyClusterCmd struct {
	Cluster *kops.Cluster

	InstanceGroups []*kops.InstanceGroup

	// NodeUpSource is the location from which we download nodeup
	NodeUpSource string

	// NodeUpHash is the sha hash
	NodeUpHash string

	// Models is a list of cloudup models to apply
	Models []string

	// TargetName specifies how we are operating e.g. direct to GCE, or AWS, or dry-run, or terraform
	TargetName string

	// Target is the fi.Target we will operate against
	Target fi.Target

	// OutDir is a local directory in which we place output, can cache files etc
	OutDir string

	// Assets is a list of sources for files (primarily when not using everything containerized)
	// Formats:
	//  raw url: http://... or https://...
	//  url with hash: <hex>@http://... or <hex>@https://...
	Assets []string

	Clientset simple.Clientset

	// DryRun is true if this is only a dry run
	DryRun bool

	MaxTaskDuration time.Duration

	// The channel we are using
	channel *kops.Channel

	// Phase can be set to a Phase to run the specific subset of tasks, if we don't want to run everything
	Phase Phase

	// LifecycleOverrides is passed in to override the lifecycle for one of more tasks.
	// The key value is the task name such as InternetGateway and the value is the fi.Lifecycle
	// that is re-mapped.
	LifecycleOverrides map[string]fi.Lifecycle

	// TaskMap is the map of tasks that we built (output)
	TaskMap map[string]fi.Task
}

func (c *ApplyClusterCmd) Run() error {
	if c.MaxTaskDuration == 0 {
		c.MaxTaskDuration = DefaultMaxTaskDuration
	}

	if c.InstanceGroups == nil {
		list, err := c.Clientset.InstanceGroupsFor(c.Cluster).List(metav1.ListOptions{})
		if err != nil {
			return err
		}
		var instanceGroups []*kops.InstanceGroup
		for i := range list.Items {
			instanceGroups = append(instanceGroups, &list.Items[i])
		}
		c.InstanceGroups = instanceGroups
	}

	if c.Models == nil {
		c.Models = CloudupModels
	}

	modelStore, err := findModelStore()
	if err != nil {
		return err
	}

	channel, err := ChannelForCluster(c.Cluster)
	if err != nil {
		return err
	}
	c.channel = channel

	stageAssetsLifecycle := fi.LifecycleSync
	securityLifecycle := fi.LifecycleSync
	networkLifecycle := fi.LifecycleSync
	clusterLifecycle := fi.LifecycleSync

	switch c.Phase {
	case Phase(""):
		// Everything ... the default

		// until we implement finding assets we need to to Ignore them
		stageAssetsLifecycle = fi.LifecycleIgnore
	case PhaseStageAssets:
		networkLifecycle = fi.LifecycleIgnore
		securityLifecycle = fi.LifecycleIgnore
		clusterLifecycle = fi.LifecycleIgnore

	case PhaseNetwork:
		stageAssetsLifecycle = fi.LifecycleIgnore
		securityLifecycle = fi.LifecycleIgnore
		clusterLifecycle = fi.LifecycleIgnore

	case PhaseSecurity:
		stageAssetsLifecycle = fi.LifecycleIgnore
		networkLifecycle = fi.LifecycleExistsAndWarnIfChanges
		clusterLifecycle = fi.LifecycleIgnore

	case PhaseCluster:
		if c.TargetName == TargetDryRun {
			stageAssetsLifecycle = fi.LifecycleIgnore
			securityLifecycle = fi.LifecycleExistsAndWarnIfChanges
			networkLifecycle = fi.LifecycleExistsAndWarnIfChanges
		} else {
			stageAssetsLifecycle = fi.LifecycleIgnore
			networkLifecycle = fi.LifecycleExistsAndValidates
			securityLifecycle = fi.LifecycleExistsAndValidates
		}

	default:
		return fmt.Errorf("unknown phase %q", c.Phase)
	}

	// This is kinda a hack.  Need to move phases out of fi.  If we use Phase here we introduce a circular
	// go dependency.
	phase := string(c.Phase)
	assetBuilder := assets.NewAssetBuilder(c.Cluster, phase)
	err = c.upgradeSpecs(assetBuilder)
	if err != nil {
		return err
	}

	err = c.validateKopsVersion()
	if err != nil {
		return err
	}

	err = c.validateKubernetesVersion()
	if err != nil {
		return err
	}

	err = validation.DeepValidate(c.Cluster, c.InstanceGroups, true)
	if err != nil {
		return err
	}

	cluster := c.Cluster

	if cluster.Spec.KubernetesVersion == "" {
		return fmt.Errorf("KubernetesVersion not set")
	}
	if cluster.Spec.DNSZone == "" && !dns.IsGossipHostname(cluster.ObjectMeta.Name) {
		return fmt.Errorf("DNSZone not set")
	}

	l := &Loader{}
	l.Init()
	l.Cluster = c.Cluster

	configBase, err := vfs.Context.BuildVfsPath(cluster.Spec.ConfigBase)
	if err != nil {
		return fmt.Errorf("error parsing config base %q: %v", cluster.Spec.ConfigBase, err)
	}

	keyStore, err := c.Clientset.KeyStore(cluster)
	if err != nil {
		return err
	}

	sshCredentialStore, err := c.Clientset.SSHCredentialStore(cluster)
	if err != nil {
		return err
	}

	secretStore, err := c.Clientset.SecretStore(cluster)
	if err != nil {
		return err
	}

	// Normalize k8s version
	versionWithoutV := strings.TrimSpace(cluster.Spec.KubernetesVersion)
	if strings.HasPrefix(versionWithoutV, "v") {
		versionWithoutV = versionWithoutV[1:]
	}
	if cluster.Spec.KubernetesVersion != versionWithoutV {
		glog.Warningf("Normalizing kubernetes version: %q -> %q", cluster.Spec.KubernetesVersion, versionWithoutV)
		cluster.Spec.KubernetesVersion = versionWithoutV
	}

	if err := c.AddFileAssets(assetBuilder); err != nil {
		return err
	}

	// Only setup transfer of kops assets if using a FileRepository
	if c.Cluster.Spec.Assets != nil && c.Cluster.Spec.Assets.FileRepository != nil {
		if err := SetKopsAssetsLocations(assetBuilder); err != nil {
			return err
		}
	}

	checkExisting := true

	l.AddTypes(map[string]interface{}{
		"keypair":        &fitasks.Keypair{},
		"secret":         &fitasks.Secret{},
		"managedFile":    &fitasks.ManagedFile{},
		"mirrorKeystore": &fitasks.MirrorKeystore{},
		"mirrorSecrets":  &fitasks.MirrorSecrets{},
	})

	cloud, err := BuildCloud(cluster)
	if err != nil {
		return err
	}

	region := ""
	project := ""

	var sshPublicKeys [][]byte
	{
		keys, err := sshCredentialStore.FindSSHPublicKeys(fi.SecretNameSSHPrimary)
		if err != nil {
			return fmt.Errorf("error retrieving SSH public key %q: %v", fi.SecretNameSSHPrimary, err)
		}

		for _, k := range keys {
			sshPublicKeys = append(sshPublicKeys, []byte(k.Spec.PublicKey))
		}
	}

	modelContext := &model.KopsModelContext{
		Cluster:        cluster,
		InstanceGroups: c.InstanceGroups,
	}

	switch kops.CloudProviderID(cluster.Spec.CloudProvider) {
	case kops.CloudProviderGCE:
		{
			gceCloud := cloud.(gce.GCECloud)
			region = gceCloud.Region()
			project = gceCloud.Project()

			if !AlphaAllowGCE.Enabled() {
				return fmt.Errorf("GCE support is currently alpha, and is feature-gated.  export KOPS_FEATURE_FLAGS=AlphaAllowGCE")
			}

			l.AddTypes(map[string]interface{}{
				"Disk":                 &gcetasks.Disk{},
				"Instance":             &gcetasks.Instance{},
				"InstanceTemplate":     &gcetasks.InstanceTemplate{},
				"Network":              &gcetasks.Network{},
				"InstanceGroupManager": &gcetasks.InstanceGroupManager{},
				"FirewallRule":         &gcetasks.FirewallRule{},
				"Address":              &gcetasks.Address{},
			})
		}

	case kops.CloudProviderDO:
		{
			if !AlphaAllowDO.Enabled() {
				return fmt.Errorf("DigitalOcean support is currently (very) alpha and is feature-gated. export KOPS_FEATURE_FLAGS=AlphaAllowDO to enable it")
			}

			modelContext.SSHPublicKeys = sshPublicKeys

			l.AddTypes(map[string]interface{}{
				"volume":  &dotasks.Volume{},
				"droplet": &dotasks.Droplet{},
			})
		}
	case kops.CloudProviderAWS:
		{
			awsCloud := cloud.(awsup.AWSCloud)
			region = awsCloud.Region()

			l.AddTypes(map[string]interface{}{
				// EC2
				"elasticIP":                   &awstasks.ElasticIP{},
				"instance":                    &awstasks.Instance{},
				"instanceElasticIPAttachment": &awstasks.InstanceElasticIPAttachment{},
				"instanceVolumeAttachment":    &awstasks.InstanceVolumeAttachment{},
				"ebsVolume":                   &awstasks.EBSVolume{},
				"sshKey":                      &awstasks.SSHKey{},

				// IAM
				"iamInstanceProfile":     &awstasks.IAMInstanceProfile{},
				"iamInstanceProfileRole": &awstasks.IAMInstanceProfileRole{},
				"iamRole":                &awstasks.IAMRole{},
				"iamRolePolicy":          &awstasks.IAMRolePolicy{},

				// VPC / Networking
				"dhcpOptions":           &awstasks.DHCPOptions{},
				"internetGateway":       &awstasks.InternetGateway{},
				"route":                 &awstasks.Route{},
				"routeTable":            &awstasks.RouteTable{},
				"routeTableAssociation": &awstasks.RouteTableAssociation{},
				"securityGroup":         &awstasks.SecurityGroup{},
				"securityGroupRule":     &awstasks.SecurityGroupRule{},
				"subnet":                &awstasks.Subnet{},
				"vpc":                   &awstasks.VPC{},
				"ngw":                   &awstasks.NatGateway{},
				"vpcDHDCPOptionsAssociation": &awstasks.VPCDHCPOptionsAssociation{},

				// ELB
				"loadBalancer":           &awstasks.LoadBalancer{},
				"loadBalancerAttachment": &awstasks.LoadBalancerAttachment{},

				// Autoscaling
				"autoscalingGroup":    &awstasks.AutoscalingGroup{},
				"launchConfiguration": &awstasks.LaunchConfiguration{},
			})

			if len(sshPublicKeys) == 0 {
				return fmt.Errorf("SSH public key must be specified when running with AWS (create with `kops create secret --name %s sshpublickey admin -i ~/.ssh/id_rsa.pub`)", cluster.ObjectMeta.Name)
			}

			modelContext.SSHPublicKeys = sshPublicKeys

			if len(sshPublicKeys) != 1 {
				return fmt.Errorf("Exactly one 'admin' SSH public key can be specified when running with AWS; please delete a key using `kops delete secret`")
			}

			l.TemplateFunctions["MachineTypeInfo"] = awsup.GetMachineTypeInfo
		}

	case kops.CloudProviderVSphere:
		{
			if !AlphaAllowVsphere.Enabled() {
				return fmt.Errorf("Vsphere support is currently alpha, and is feature-gated.  export KOPS_FEATURE_FLAGS=AlphaAllowVsphere")
			}

			vsphereCloud := cloud.(*vsphere.VSphereCloud)
			// TODO: map region with vCenter cluster, or datacenter, or datastore?
			region = vsphereCloud.Cluster

			l.AddTypes(map[string]interface{}{
				"instance": &vspheretasks.VirtualMachine{},
			})
		}

	case kops.CloudProviderBareMetal:
		{
			if !AlphaAllowBareMetal.Enabled() {
				return fmt.Errorf("BareMetal support is currently (very) alpha and is feature-gated. export KOPS_FEATURE_FLAGS=AlphaAllowBareMetal to enable it")
			}

			// No additional tasks (yet)
		}

	case kops.CloudProviderOpenstack:
		{
			osCloud := cloud.(openstack.OpenstackCloud)
			region = osCloud.Region()

			l.AddTypes(map[string]interface{}{
				// Networking
				"network": &openstacktasks.Network{},
				"router":  &openstacktasks.Router{},
			})
		}
	default:
		return fmt.Errorf("unknown CloudProvider %q", cluster.Spec.CloudProvider)
	}

	modelContext.Region = region

	if dns.IsGossipHostname(cluster.ObjectMeta.Name) {
		glog.Infof("Gossip DNS: skipping DNS validation")
	} else {
		err = validateDNS(cluster, cloud)
		if err != nil {
			return err
		}
	}

	clusterTags, err := buildCloudupTags(cluster)
	if err != nil {
		return err
	}

	tf := &TemplateFunctions{
		cluster:        cluster,
		instanceGroups: c.InstanceGroups,
		tags:           clusterTags,
		region:         region,
		modelContext:   modelContext,
	}

	l.Tags = clusterTags
	l.WorkDir = c.OutDir
	l.ModelStore = modelStore

	var fileModels []string
	for _, m := range c.Models {
		switch m {
		case "proto":
		// No proto code options; no file model

		case "cloudup":
			templates, err := templates.LoadTemplates(cluster, models.NewAssetPath("cloudup/resources"))
			if err != nil {
				return fmt.Errorf("error loading templates: %v", err)
			}
			tf.AddTo(templates.TemplateFunctions)

			l.Builders = append(l.Builders,
				&BootstrapChannelBuilder{
					cluster:      cluster,
					Lifecycle:    &clusterLifecycle,
					templates:    templates,
					assetBuilder: assetBuilder,
				},
				&model.PKIModelBuilder{KopsModelContext: modelContext, Lifecycle: &clusterLifecycle},
			)

			switch kops.CloudProviderID(cluster.Spec.CloudProvider) {
			case kops.CloudProviderAWS:
				awsModelContext := &awsmodel.AWSModelContext{
					KopsModelContext: modelContext,
				}

				l.Builders = append(l.Builders,
					&model.MasterVolumeBuilder{KopsModelContext: modelContext, Lifecycle: &clusterLifecycle},
					&awsmodel.APILoadBalancerBuilder{AWSModelContext: awsModelContext, Lifecycle: &clusterLifecycle, SecurityLifecycle: &securityLifecycle},
					&model.BastionModelBuilder{KopsModelContext: modelContext, Lifecycle: &clusterLifecycle, SecurityLifecycle: &securityLifecycle},
					&model.DNSModelBuilder{KopsModelContext: modelContext, Lifecycle: &clusterLifecycle},
					&model.ExternalAccessModelBuilder{KopsModelContext: modelContext, Lifecycle: &securityLifecycle},
					&model.FirewallModelBuilder{KopsModelContext: modelContext, Lifecycle: &securityLifecycle},
					&model.SSHKeyModelBuilder{KopsModelContext: modelContext, Lifecycle: &securityLifecycle},
				)

				l.Builders = append(l.Builders,
					&model.NetworkModelBuilder{KopsModelContext: modelContext, Lifecycle: &networkLifecycle},
				)

				l.Builders = append(l.Builders,
					&model.IAMModelBuilder{KopsModelContext: modelContext, Lifecycle: &securityLifecycle},
				)
			case kops.CloudProviderDO:
				l.Builders = append(l.Builders,
					&model.MasterVolumeBuilder{KopsModelContext: modelContext, Lifecycle: &clusterLifecycle},
				)

			case kops.CloudProviderGCE:
				gceModelContext := &gcemodel.GCEModelContext{
					KopsModelContext: modelContext,
				}

				storageAclLifecycle := securityLifecycle
				if storageAclLifecycle != fi.LifecycleIgnore {
					// This is a best-effort permissions fix
					storageAclLifecycle = fi.LifecycleWarnIfInsufficientAccess
				}

				l.Builders = append(l.Builders,
					&model.MasterVolumeBuilder{KopsModelContext: modelContext, Lifecycle: &clusterLifecycle},

					&gcemodel.APILoadBalancerBuilder{GCEModelContext: gceModelContext, Lifecycle: &securityLifecycle},
					&gcemodel.ExternalAccessModelBuilder{GCEModelContext: gceModelContext, Lifecycle: &securityLifecycle},
					&gcemodel.FirewallModelBuilder{GCEModelContext: gceModelContext, Lifecycle: &securityLifecycle},
					&gcemodel.NetworkModelBuilder{GCEModelContext: gceModelContext, Lifecycle: &networkLifecycle},
				)

				if featureflag.GoogleCloudBucketAcl.Enabled() {
					l.Builders = append(l.Builders,
						&gcemodel.StorageAclBuilder{GCEModelContext: gceModelContext, Cloud: cloud.(gce.GCECloud), Lifecycle: &storageAclLifecycle},
					)
				}

			case kops.CloudProviderVSphere:
				// No special settings (yet!)

			case kops.CloudProviderBareMetal:
				// No special settings (yet!)

			case kops.CloudProviderOpenstack:
				openstackModelContext := &openstackmodel.OpenstackModelContext{
					KopsModelContext: modelContext,
				}

				l.Builders = append(l.Builders,
					&openstackmodel.NetworkModelBuilder{OpenstackModelContext: openstackModelContext, Lifecycle: &networkLifecycle},
				)

			default:
				return fmt.Errorf("unknown cloudprovider %q", cluster.Spec.CloudProvider)
			}

			fileModels = append(fileModels, m)

		default:
			fileModels = append(fileModels, m)
		}
	}

	l.TemplateFunctions["CA"] = func() fi.CAStore {
		return keyStore
	}
	l.TemplateFunctions["Secrets"] = func() fi.SecretStore {
		return secretStore
	}

	bootstrapScriptBuilder := &model.BootstrapScript{
		NodeUpConfigBuilder: func(ig *kops.InstanceGroup) (*nodeup.Config, error) { return c.BuildNodeUpConfig(assetBuilder, ig) },
		NodeUpSource:        c.NodeUpSource,
		NodeUpSourceHash:    c.NodeUpHash,
	}
	switch kops.CloudProviderID(cluster.Spec.CloudProvider) {
	case kops.CloudProviderAWS:
		awsModelContext := &awsmodel.AWSModelContext{
			KopsModelContext: modelContext,
		}

		l.Builders = append(l.Builders, &awsmodel.AutoscalingGroupModelBuilder{
			AWSModelContext: awsModelContext,
			BootstrapScript: bootstrapScriptBuilder,
			Lifecycle:       &clusterLifecycle,

			SecurityLifecycle: &securityLifecycle,
		})
	case kops.CloudProviderDO:
		doModelContext := &domodel.DOModelContext{
			KopsModelContext: modelContext,
		}

		l.Builders = append(l.Builders, &domodel.DropletBuilder{
			DOModelContext:  doModelContext,
			BootstrapScript: bootstrapScriptBuilder,
			Lifecycle:       &clusterLifecycle,
		})
	case kops.CloudProviderGCE:
		{
			gceModelContext := &gcemodel.GCEModelContext{
				KopsModelContext: modelContext,
			}

			l.Builders = append(l.Builders, &gcemodel.AutoscalingGroupModelBuilder{
				GCEModelContext: gceModelContext,
				BootstrapScript: bootstrapScriptBuilder,
				Lifecycle:       &clusterLifecycle,
			})
		}
	case kops.CloudProviderVSphere:
		{
			vsphereModelContext := &vspheremodel.VSphereModelContext{
				KopsModelContext: modelContext,
			}

			l.Builders = append(l.Builders, &vspheremodel.AutoscalingGroupModelBuilder{
				VSphereModelContext: vsphereModelContext,
				BootstrapScript:     bootstrapScriptBuilder,
				Lifecycle:           &clusterLifecycle,
			})
		}

	case kops.CloudProviderBareMetal:
		// BareMetal tasks will go here

	case kops.CloudProviderOpenstack:

	default:
		return fmt.Errorf("unknown cloudprovider %q", cluster.Spec.CloudProvider)
	}

	l.TemplateFunctions["Masters"] = tf.modelContext.MasterInstanceGroups

	tf.AddTo(l.TemplateFunctions)

	taskMap, err := l.BuildTasks(modelStore, fileModels, assetBuilder, &stageAssetsLifecycle, c.LifecycleOverrides)
	if err != nil {
		return fmt.Errorf("error building tasks: %v", err)
	}

	c.TaskMap = taskMap

	var target fi.Target
	dryRun := false
	shouldPrecreateDNS := true

	switch c.TargetName {
	case TargetDirect:
		switch kops.CloudProviderID(cluster.Spec.CloudProvider) {
		case kops.CloudProviderGCE:
			target = gce.NewGCEAPITarget(cloud.(gce.GCECloud))
		case kops.CloudProviderAWS:
			target = awsup.NewAWSAPITarget(cloud.(awsup.AWSCloud))
		case kops.CloudProviderDO:
			target = do.NewDOAPITarget(cloud.(*digitalocean.Cloud))
		case kops.CloudProviderVSphere:
			target = vsphere.NewVSphereAPITarget(cloud.(*vsphere.VSphereCloud))
		case kops.CloudProviderBareMetal:
			target = baremetal.NewTarget(cloud.(*baremetal.Cloud))
		case kops.CloudProviderOpenstack:
			target = openstack.NewOpenstackAPITarget(cloud.(openstack.OpenstackCloud))
		default:
			return fmt.Errorf("direct configuration not supported with CloudProvider:%q", cluster.Spec.CloudProvider)
		}

	case TargetTerraform:
		checkExisting = false
		outDir := c.OutDir
		tf := terraform.NewTerraformTarget(cloud, region, project, outDir, cluster.Spec.Target)

		// We include a few "util" variables in the TF output
		if err := tf.AddOutputVariable("region", terraform.LiteralFromStringValue(region)); err != nil {
			return err
		}

		if project != "" {
			if err := tf.AddOutputVariable("project", terraform.LiteralFromStringValue(project)); err != nil {
				return err
			}
		}

		if err := tf.AddOutputVariable("cluster_name", terraform.LiteralFromStringValue(cluster.ObjectMeta.Name)); err != nil {
			return err
		}

		target = tf

		// Can cause conflicts with terraform management
		shouldPrecreateDNS = false

	case TargetCloudformation:
		checkExisting = false
		outDir := c.OutDir
		target = cloudformation.NewCloudformationTarget(cloud, region, project, outDir)

		// Can cause conflicts with cloudformation management
		shouldPrecreateDNS = false

	case TargetDryRun:
		target = fi.NewDryRunTarget(assetBuilder, os.Stdout)
		dryRun = true

		// Avoid making changes on a dry-run
		shouldPrecreateDNS = false

	default:
		return fmt.Errorf("unsupported target type %q", c.TargetName)
	}
	c.Target = target

	if !dryRun {
		err = registry.WriteConfigDeprecated(cluster, configBase.Join(registry.PathClusterCompleted), c.Cluster)
		if err != nil {
			return fmt.Errorf("error writing completed cluster spec: %v", err)
		}

		vfsMirror := vfsclientset.NewInstanceGroupMirror(cluster, configBase)

		for _, g := range c.InstanceGroups {
			// TODO: We need to update the mirror (below), but do we need to update the primary?
			_, err := c.Clientset.InstanceGroupsFor(c.Cluster).Update(g)
			if err != nil {
				return fmt.Errorf("error writing InstanceGroup %q to registry: %v", g.ObjectMeta.Name, err)
			}

			// TODO: Don't write if vfsMirror == c.ClientSet
			if err := vfsMirror.WriteMirror(g); err != nil {
				return fmt.Errorf("error writing instance group spec to mirror: %v", err)
			}
		}
	}

	context, err := fi.NewContext(target, cluster, cloud, keyStore, secretStore, configBase, checkExisting, taskMap)
	if err != nil {
		return fmt.Errorf("error building context: %v", err)
	}
	defer context.Close()

	err = context.RunTasks(c.MaxTaskDuration)
	if err != nil {
		return fmt.Errorf("error running tasks: %v", err)
	}

	if dns.IsGossipHostname(cluster.Name) {
		shouldPrecreateDNS = false
	}

	if shouldPrecreateDNS {
		if err := precreateDNS(cluster, cloud); err != nil {
			glog.Warningf("unable to pre-create DNS records - cluster startup may be slower: %v", err)
		}
	}

	err = target.Finish(taskMap) //This will finish the apply, and print the changes
	if err != nil {
		return fmt.Errorf("error closing target: %v", err)
	}

	return nil
}

// upgradeSpecs ensures that fields are fully populated / defaulted
func (c *ApplyClusterCmd) upgradeSpecs(assetBuilder *assets.AssetBuilder) error {
	fullCluster, err := PopulateClusterSpec(c.Clientset, c.Cluster, assetBuilder)
	if err != nil {
		return err
	}
	c.Cluster = fullCluster

	for i, g := range c.InstanceGroups {
		fullGroup, err := PopulateInstanceGroupSpec(fullCluster, g, c.channel)
		if err != nil {
			return err
		}
		c.InstanceGroups[i] = fullGroup
	}

	return nil
}

// validateKopsVersion ensures that kops meet the version requirements / recommendations in the channel
func (c *ApplyClusterCmd) validateKopsVersion() error {
	kopsVersion, err := semver.ParseTolerant(kopsbase.Version)
	if err != nil {
		glog.Warningf("unable to parse kops version %q", kopsbase.Version)
		// Not a hard-error
		return nil
	}

	versionInfo := kops.FindKopsVersionSpec(c.channel.Spec.KopsVersions, kopsVersion)
	if versionInfo == nil {
		glog.Warningf("unable to find version information for kops version %q in channel", kopsVersion)
		// Not a hard-error
		return nil
	}

	recommended, err := versionInfo.FindRecommendedUpgrade(kopsVersion)
	if err != nil {
		glog.Warningf("unable to parse version recommendation for kops version %q in channel", kopsVersion)
	}

	required, err := versionInfo.IsUpgradeRequired(kopsVersion)
	if err != nil {
		glog.Warningf("unable to parse version requirement for kops version %q in channel", kopsVersion)
	}

	if recommended != nil && !required {
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
		fmt.Printf("A new kops version is available: %s\n", recommended)
		fmt.Printf("\n")
		fmt.Printf("Upgrading is recommended\n")
		fmt.Printf("More information: %s\n", buildPermalink("upgrade_kops", recommended.String()))
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
	} else if required {
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
		if recommended != nil {
			fmt.Printf("A new kops version is available: %s\n", recommended)
		}
		fmt.Printf("\n")
		fmt.Printf("This version of kops (%s) is no longer supported; upgrading is required\n", kopsbase.Version)
		fmt.Printf("(you can bypass this check by exporting KOPS_RUN_OBSOLETE_VERSION)\n")
		fmt.Printf("\n")
		fmt.Printf("More information: %s\n", buildPermalink("upgrade_kops", recommended.String()))
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
	}

	if required {
		if os.Getenv("KOPS_RUN_OBSOLETE_VERSION") == "" {
			return fmt.Errorf("kops upgrade is required")
		}
	}

	return nil
}

// validateKubernetesVersion ensures that kubernetes meet the version requirements / recommendations in the channel
func (c *ApplyClusterCmd) validateKubernetesVersion() error {
	parsed, err := util.ParseKubernetesVersion(c.Cluster.Spec.KubernetesVersion)
	if err != nil {
		glog.Warningf("unable to parse kubernetes version %q", c.Cluster.Spec.KubernetesVersion)
		// Not a hard-error
		return nil
	}

	// TODO: make util.ParseKubernetesVersion not return a pointer
	kubernetesVersion := *parsed

	versionInfo := kops.FindKubernetesVersionSpec(c.channel.Spec.KubernetesVersions, kubernetesVersion)
	if versionInfo == nil {
		glog.Warningf("unable to find version information for kubernetes version %q in channel", kubernetesVersion)
		// Not a hard-error
		return nil
	}

	recommended, err := versionInfo.FindRecommendedUpgrade(kubernetesVersion)
	if err != nil {
		glog.Warningf("unable to parse version recommendation for kubernetes version %q in channel", kubernetesVersion)
	}

	required, err := versionInfo.IsUpgradeRequired(kubernetesVersion)
	if err != nil {
		glog.Warningf("unable to parse version requirement for kubernetes version %q in channel", kubernetesVersion)
	}

	if recommended != nil && !required {
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
		fmt.Printf("A new kubernetes version is available: %s\n", recommended)
		fmt.Printf("Upgrading is recommended (try kops upgrade cluster)\n")
		fmt.Printf("\n")
		fmt.Printf("More information: %s\n", buildPermalink("upgrade_k8s", recommended.String()))
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
	} else if required {
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
		if recommended != nil {
			fmt.Printf("A new kubernetes version is available: %s\n", recommended)
		}
		fmt.Printf("\n")
		fmt.Printf("This version of kubernetes is no longer supported; upgrading is required\n")
		fmt.Printf("(you can bypass this check by exporting KOPS_RUN_OBSOLETE_VERSION)\n")
		fmt.Printf("\n")
		fmt.Printf("More information: %s\n", buildPermalink("upgrade_k8s", recommended.String()))
		fmt.Printf("\n")
		fmt.Printf(starline)
		fmt.Printf("\n")
	}

	if required {
		if os.Getenv("KOPS_RUN_OBSOLETE_VERSION") == "" {
			return fmt.Errorf("kubernetes upgrade is required")
		}
	}

	return nil
}

// AddFileAssets adds the file assets within the assetBuilder
func (c *ApplyClusterCmd) AddFileAssets(assetBuilder *assets.AssetBuilder) error {

	var baseURL string
	var err error
	if components.IsBaseURL(c.Cluster.Spec.KubernetesVersion) {
		baseURL = c.Cluster.Spec.KubernetesVersion
	} else {
		baseURL = "https://storage.googleapis.com/kubernetes-release/release/v" + c.Cluster.Spec.KubernetesVersion
	}

	k8sAssetsNames := []string{
		"/bin/linux/amd64/kubelet",
		"/bin/linux/amd64/kubectl",
	}
	if needsMounterAsset(c.Cluster, c.InstanceGroups) {
		k8sVersion, err := util.ParseKubernetesVersion(c.Cluster.Spec.KubernetesVersion)
		if err != nil {
			return fmt.Errorf("unable to determine kubernetes version from %q", c.Cluster.Spec.KubernetesVersion)
		} else if util.IsKubernetesGTE("1.9", *k8sVersion) {
			// Available directly
			k8sAssetsNames = append(k8sAssetsNames, "/bin/linux/amd64/mounter")
		} else {
			// Only available in the kubernetes-manifests.tar.gz directory
			k8sAssetsNames = append(k8sAssetsNames, "/kubernetes-manifests.tar.gz")
		}
	}

	for _, a := range k8sAssetsNames {
		k, err := url.Parse(baseURL)
		if err != nil {
			return err
		}
		k.Path = path.Join(k.Path, a)

		u, hash, err := assetBuilder.RemapFileAndSHA(k)
		if err != nil {
			return err
		}
		c.Assets = append(c.Assets, hash.Hex()+"@"+u.String())
	}

	if usesCNI(c.Cluster) {
		cniAsset, cniAssetHashString, err := findCNIAssets(c.Cluster, assetBuilder)
		if err != nil {
			return err
		}

		c.Assets = append(c.Assets, cniAssetHashString+"@"+cniAsset.String())
	}

	// TODO figure out if we can only do this for CoreOS only and GCE Container OS
	// TODO It is very difficult to pre-determine what OS an ami is, and if that OS needs socat
	// At this time we just copy the socat binary to all distros.  Most distros will be there own
	// socat binary.  Container operating systems like CoreOS need to have socat added to them.
	{
		utilsLocation, hash, err := KopsFileUrl("linux/amd64/utils.tar.gz", assetBuilder)
		if err != nil {
			return err
		}
		c.Assets = append(c.Assets, hash.Hex()+"@"+utilsLocation.String())
	}

	n, hash, err := NodeUpLocation(assetBuilder)
	if err != nil {
		return err
	}
	c.NodeUpSource = n.String()
	c.NodeUpHash = hash.Hex()

	// Explicitly add the protokube image,
	// otherwise when the Target is DryRun this asset is not added
	// Is there a better way to call this?
	_, _, err = ProtokubeImageSource(assetBuilder)
	if err != nil {
		return err
	}

	return nil
}

// buildPermalink returns a link to our "permalink docs", to further explain an error message
func buildPermalink(key, anchor string) string {
	url := "https://github.com/kubernetes/kops/blob/master/permalinks/" + key + ".md"
	if anchor != "" {
		url += "#" + anchor
	}
	return url
}

func ChannelForCluster(c *kops.Cluster) (*kops.Channel, error) {
	channelLocation := c.Spec.Channel
	if channelLocation == "" {
		channelLocation = kops.DefaultChannel
	}
	return kops.LoadChannel(channelLocation)
}

// needsMounterAsset checks if we need the mounter program
// This is only needed currently on ContainerOS i.e. GCE, but we don't have a nice way to detect it yet
func needsMounterAsset(c *kops.Cluster, instanceGroups []*kops.InstanceGroup) bool {
	// TODO: Do real detection of ContainerOS (but this has to work with image names, and maybe even forked images)
	switch kops.CloudProviderID(c.Spec.CloudProvider) {
	case kops.CloudProviderGCE:
		return true
	default:
		return false
	}
}

// BuildNodeUpConfig returns the NodeUp config, in YAML format
func (c *ApplyClusterCmd) BuildNodeUpConfig(assetBuilder *assets.AssetBuilder, ig *kops.InstanceGroup) (*nodeup.Config, error) {
	if ig == nil {
		return nil, fmt.Errorf("instanceGroup cannot be nil")
	}

	cluster := c.Cluster

	configBase, err := vfs.Context.BuildVfsPath(cluster.Spec.ConfigBase)
	if err != nil {
		return nil, fmt.Errorf("error parsing config base %q: %v", cluster.Spec.ConfigBase, err)
	}

	// TODO: Remove
	clusterTags, err := buildCloudupTags(cluster)
	if err != nil {
		return nil, err
	}

	channels := []string{
		configBase.Join("addons", "bootstrap-channel.yaml").Path(),
	}

	for i := range c.Cluster.Spec.Addons {
		channels = append(channels, c.Cluster.Spec.Addons[i].Manifest)
	}

	role := ig.Spec.Role
	if role == "" {
		return nil, fmt.Errorf("cannot determine role for instance group: %v", ig.ObjectMeta.Name)
	}

	nodeUpTags, err := buildNodeupTags(role, cluster, clusterTags)
	if err != nil {
		return nil, err
	}

	config := &nodeup.Config{}
	for _, tag := range nodeUpTags.List() {
		config.Tags = append(config.Tags, tag)
	}

	config.Assets = c.Assets
	config.ClusterName = cluster.ObjectMeta.Name
	config.ConfigBase = fi.String(configBase.Path())
	config.InstanceGroupName = ig.ObjectMeta.Name

	var images []*nodeup.Image

	if components.IsBaseURL(cluster.Spec.KubernetesVersion) {
		// When using a custom version, we want to preload the images over http
		components := []string{"kube-proxy"}
		if role == kops.InstanceGroupRoleMaster {
			components = append(components, "kube-apiserver", "kube-controller-manager", "kube-scheduler")
		}

		for _, component := range components {
			baseURL, err := url.Parse(c.Cluster.Spec.KubernetesVersion)
			if err != nil {
				return nil, err
			}

			baseURL.Path = path.Join(baseURL.Path, "/bin/linux/amd64/", component+".tar")

			u, hash, err := assetBuilder.RemapFileAndSHA(baseURL)
			if err != nil {
				return nil, err
			}

			image := &nodeup.Image{
				Source: u.String(),
				Hash:   hash.Hex(),
			}
			images = append(images, image)
		}
	}

	{
		location, hash, err := ProtokubeImageSource(assetBuilder)
		if err != nil {
			return nil, err
		}

		config.ProtokubeImage = &nodeup.Image{
			Name:   kopsbase.DefaultProtokubeImageName(),
			Source: location.String(),
			Hash:   hash.Hex(),
		}
	}

	config.Images = images
	config.Channels = channels

	return config, nil
}