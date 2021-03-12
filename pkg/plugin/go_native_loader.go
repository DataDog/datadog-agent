package plugin

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"plugin"
	"strings"

	pluginApi "github.com/DataDog/datadog-agent/pkg/api/plugin"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckFactory is an alias for a check factory function in a plugin
type CheckFactory func() pluginApi.Check

// GoNativePluginCheckLoader is a structure to hold our loaded check factories from
// plugins
type GoNativePluginCheckLoader struct {
	checks map[string]CheckFactory
}

type pluginLibrary interface {
	Lookup(string) (plugin.Symbol, error)
}

// PluginPathSuffix is a path that will be added to the current working directory as
// a plugin library repository
const PluginPathSuffix = "go-native-plugins"

func init() {
	loaderFactory := func() (check.Loader, error) {
		loader, err := NewGoNativePluginCheckLoader()
		if err != nil {
			log.Error(err)
		}

		return loader, nil
	}

	loaders.RegisterLoader(10, loaderFactory)
}

func loadPlugins(filePaths []string) (map[string]pluginLibrary, error) {
	pluginLibraries := map[string]pluginLibrary{}
	for _, filePath := range filePaths {
		fileName := path.Base(filePath)
		if !strings.HasSuffix(fileName, ".so") {
			log.Warnf("Ignoring '%s' - wrong extension ", fileName)
			continue
		}

		pluginObj, err := plugin.Open(filePath)
		if err != nil {
			log.Error(err)
			continue
		}

		log.Warnf("*** Loaded plugin '%s' ***", fileName)

		pluginLibraries[fileName[:len(fileName)-3]] = pluginObj
	}

	return pluginLibraries, nil
}

func getPluginFiles(pluginDir string) (map[string]pluginLibrary, error) {
	_, err := os.Stat(pluginDir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("Plugin dir '%s' not found", pluginDir)
	}

	files, err := ioutil.ReadDir(pluginDir)
	if err != nil {
		return nil, err
	}

	filePaths := []string{}
	for _, file := range files {
		filePaths = append(filePaths, path.Join(pluginDir, file.Name()))
	}

	return loadPlugins(filePaths)
}

func (loader *GoNativePluginCheckLoader) symbol(
	library pluginLibrary,
	symbolName string,
) (plugin.Symbol, error) {

	symbol, err := library.Lookup(symbolName)
	if err != nil {
		return nil, err
	}

	return symbol, nil
}

func (loader *GoNativePluginCheckLoader) loadPlugin(
	library pluginLibrary,
) (map[string]string, CheckFactory, error) {
	infoSymbol, err := loader.symbol(library, "PluginInfo")
	if err != nil {
		return nil, nil, err
	}

	info, ok := infoSymbol.(func() map[string]string)
	if !ok {
		log.Errorf("Could not cast PluginInfo symbol: %s", err)
		return nil, nil, err
	}

	factorySymbol, err := loader.symbol(library, "NewCheck")
	if err != nil {
		return nil, nil, err
	}

	checkFactoryFunc, ok := factorySymbol.(func() pluginApi.Check)
	if !ok {
		log.Errorf("Could not cast NewCheck symbol: %s", err)
		return nil, nil, err
	}

	return info(), checkFactoryFunc, nil
}

// NewGoNativePluginCheckLoader creates a new loader object for native Golang plugins
func NewGoNativePluginCheckLoader() (*GoNativePluginCheckLoader, error) {
	currDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf(
			"golang-native-plugin.loader: Could not get current working dir: %s",
			err,
		)
	}

	loadingPath := path.Join(currDir, PluginPathSuffix)
	log.Infof("GoNativePluginCheckLoader using path: %s", loadingPath)

	loader := &GoNativePluginCheckLoader{
		checks: map[string]CheckFactory{},
	}

	return loader, loader.loadAllPlugins(loadingPath)
}

func (loader *GoNativePluginCheckLoader) loadAllPlugins(pluginDir string) error {
	log.Warnf("Loading all plugins in %s", pluginDir)

	pluginLibraries, err := getPluginFiles(pluginDir)
	if err != nil {
		log.Warn(err)
		return err
	}

	for pluginFsName, pluginLibrary := range pluginLibraries {
		log.Warnf("   -> Caching: %s: %v", pluginFsName, pluginLibrary)

		pluginInfo, newCheckFunc, err := loader.loadPlugin(pluginLibrary)
		if err != nil {
			return err
		}

		pluginName := pluginInfo["id"]

		log.Warnf(
			"****** Loaded plugin >>>>> %s@%s <<<<< *****",
			pluginName,
			pluginInfo["version"],
		)

		loader.checks[pluginName] = newCheckFunc
	}

	return nil
}

// Load returns a Go check given a configuration object and its initialization data
func (loader *GoNativePluginCheckLoader) Load(
	config integration.Config,
	instance integration.Data,
) (check.Check, error) {

	factory, found := loader.checks[config.Name]
	if !found {
		return nil, fmt.Errorf(
			"golang-native-plugin.loader: Check %s not found in Golang native loader",
			config.Name,
		)
	}

	log.Warnf("golang-native-plugin.loader: Found plugin %s - attempting to load it...", config.Name)

	c := NewPluginCheckAdapter(factory())
	if err := c.Configure(instance, config.InitConfig, config.Source); err != nil {
		return nil, fmt.Errorf(
			"golang-native-plugin.loader: Could not configure check %s: %s",
			c,
			err,
		)
	}

	return c, nil
}

// Name return returns Go loader name
func (loader *GoNativePluginCheckLoader) Name() string {
	return "golang-native-plugin"
}

func (loader *GoNativePluginCheckLoader) String() string {
	return "Go Native Plugin Check Loader"
}
