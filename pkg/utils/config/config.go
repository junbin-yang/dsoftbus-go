package config

import (
	"flag"
	"fmt"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
)

var (
	APPNAME    string = "dsoftbus"
	VERSION    string = "undefined"
	BUILD_TIME string = "undefined"
	GO_VERSION string = "undefined"
)

type Config struct {
	DeviceType string
	DeviceName string
	UUID       string
	Interface  string
	Logger     struct {
		Dir    string
		Level  string
		Rotate bool
	}
}

func init() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stdout, APPNAME+", version: "+VERSION+" (built at "+BUILD_TIME+") "+GO_VERSION)
		flag.PrintDefaults()
	}
	flag.Parse()
}

func Parse() *Config {
	ex, e := os.Executable()
	if e != nil {
		panic(e)
	}

	cfile := filepath.Dir(ex) + "/" + APPNAME + ".yml"
	if _, err := os.Stat(cfile); os.IsNotExist(err) {
		cfile = "/etc/" + APPNAME + ".yml"
	}

	conf := new(Config)
	data, err := ioutil.ReadFile(cfile)
	if err != nil {
		panic(err)
	}
	yaml.Unmarshal(data, &conf)

	defer log.Sync()
	if conf.Logger.Rotate {
		if len(conf.Logger.Dir) == 0 {
			conf.Logger.Dir = filepath.Dir(ex)
		}
		out := log.NewProductionRotateByTime(conf.Logger.Dir + "/" + APPNAME + ".log")
		logger := log.New(out, log.InfoLevel)
		log.ReplaceDefault(logger)
	}
	switch conf.Logger.Level {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	return conf
}
