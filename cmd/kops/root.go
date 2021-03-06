package main

import (
	goflag "flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"k8s.io/kops/cmd/kops/util"
	kopsapi "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/kops/v1alpha1"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/upup/pkg/kutil"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	"os"
)

type Factory interface {
	Clientset() (simple.Clientset, error)
}

type RootCmd struct {
	util.FactoryOptions

	factory *util.Factory

	configFile string

	clusterName string

	cobraCommand *cobra.Command
}

var _ Factory = &RootCmd{}

var rootCommand = RootCmd{
	cobraCommand: &cobra.Command{
		Use:   "kops",
		Short: "kops is kubernetes ops",
		Long: `kops is kubernetes ops.
It allows you to create, destroy, upgrade and maintain clusters.`,
	},
}

func Execute() {
	if err := initializeSchemas(); err != nil {
		exitWithError(fmt.Errorf("initialization error: %v", err))
	}

	goflag.Set("logtostderr", "true")
	goflag.CommandLine.Parse([]string{})
	if err := rootCommand.cobraCommand.Execute(); err != nil {
		exitWithError(err)
	}
}

func initializeSchemas() error {
	scheme := k8sapi.Scheme //runtime.NewScheme()
	if err := kopsapi.AddToScheme(scheme); err != nil {
		return err
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return err
	}
	return nil
}

func init() {
	cobra.OnInitialize(initConfig)

	factory := util.NewFactory(&rootCommand.FactoryOptions)
	rootCommand.factory = factory

	NewCmdRoot(factory, os.Stdout)
}

func NewCmdRoot(f *util.Factory, out io.Writer) *cobra.Command {
	//options := &RootOptions{}

	cmd := rootCommand.cobraCommand

	cmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)

	cmd.PersistentFlags().StringVar(&rootCommand.configFile, "config", "", "config file (default is $HOME/.kops.yaml)")

	defaultStateStore := os.Getenv("KOPS_STATE_STORE")
	cmd.PersistentFlags().StringVarP(&rootCommand.RegistryPath, "state", "", defaultStateStore, "Location of state storage")

	cmd.PersistentFlags().StringVarP(&rootCommand.clusterName, "name", "", "", "Name of cluster")

	// create subcommands
	cmd.AddCommand(NewCmdCreate(f, out))
	cmd.AddCommand(NewCmdEdit(f, out))
	cmd.AddCommand(NewCmdUpdate(f, out))

	return cmd
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if rootCommand.configFile != "" {
		// enable ability to specify config file via flag
		viper.SetConfigFile(rootCommand.configFile)
	}

	viper.SetConfigName(".kops") // name of config file (without extension)
	viper.AddConfigPath("$HOME") // adding home directory as first search path
	viper.AutomaticEnv()         // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func (c *RootCmd) AddCommand(cmd *cobra.Command) {
	c.cobraCommand.AddCommand(cmd)
}

// ProcessArgs will parse the positional args.  It assumes one of these formats:
//  * <no arguments at all>
//  * <clustername> (and --name not specified)
// Everything else is an error.
func (c *RootCmd) ProcessArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 {
		// Assume <clustername>
		if c.clusterName != "" {
			return fmt.Errorf("Cannot specify cluster via --name and positional argument")
		}
		c.clusterName = args[0]
		return nil
	}

	return fmt.Errorf("expected a single <clustername> to be passed as an argument")
}

func (c *RootCmd) ClusterName() string {
	if c.clusterName != "" {
		return c.clusterName
	}

	// Read from kubeconfig
	pathOptions := clientcmd.NewDefaultPathOptions()

	config, err := pathOptions.GetStartingConfig()
	if err != nil {
		glog.Warningf("error reading kubecfg: %v", err)
	} else if config.CurrentContext == "" {
		glog.Warningf("no context set in kubecfg")
	} else {
		context := config.Contexts[config.CurrentContext]
		if context == nil {
			glog.Warningf("context %q in kubecfg not found", config.CurrentContext)
		} else if context.Cluster == "" {
			glog.Warningf("context %q in kubecfg did not have a cluster", config.CurrentContext)
		} else {
			fmt.Fprintf(os.Stderr, "Using cluster from kubectl context: %s\n\n", context.Cluster)
			c.clusterName = context.Cluster
		}
	}

	//config, err := readKubectlClusterConfig()
	//if err != nil {
	//	glog.Warningf("error reading kubecfg: %v", err)
	//} else if config != nil && config.Name != "" {
	//	fmt.Fprintf(os.Stderr, "Using cluster from kubectl context: %s\n\n", config.Name)
	//	c.clusterName = config.Name
	//}

	return c.clusterName
}

func readKubectlClusterConfig() (*kutil.KubectlClusterWithName, error) {
	kubectl := &kutil.Kubectl{}
	context, err := kubectl.GetCurrentContext()
	if err != nil {
		return nil, fmt.Errorf("error getting current context from kubectl: %v", err)
	}
	glog.V(4).Infof("context = %q", context)

	config, err := kubectl.GetConfig(true)
	if err != nil {
		return nil, fmt.Errorf("error getting current config from kubectl: %v", err)
	}

	// Minify should have done this
	if len(config.Clusters) != 1 {
		return nil, fmt.Errorf("expected exactly one cluster in kubectl config, found %d", len(config.Clusters))
	}

	return config.Clusters[0], nil
}

func (c *RootCmd) Clientset() (simple.Clientset, error) {
	return c.factory.Clientset()
}

func (c *RootCmd) Cluster() (*kopsapi.Cluster, error) {
	clientset, err := c.Clientset()
	if err != nil {
		return nil, err
	}

	clusterName := c.ClusterName()
	if clusterName == "" {
		return nil, fmt.Errorf("--name is required")
	}

	cluster, err := clientset.Clusters().Get(clusterName)
	if err != nil {
		return nil, fmt.Errorf("error reading cluster configuration: %v", err)
	}
	if cluster == nil {
		return nil, fmt.Errorf("cluster %q not found", clusterName)
	}

	if clusterName != cluster.Name {
		return nil, fmt.Errorf("cluster name did not match expected name: %v vs %v", clusterName, cluster.Name)
	}
	return cluster, nil
}
