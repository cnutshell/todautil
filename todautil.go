package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/chaos-mesh/chaos-mesh/pkg/bpm"
	"github.com/chaos-mesh/chaos-mesh/pkg/log"
	jrpc "github.com/ethereum/go-ethereum/rpc"
)

const (
	todaBin = "toda"
)

var (
	conf string
	path string
)

func init() {
	flag.StringVar(&conf, "conf", "", "config file")
	flag.StringVar(&path, "path", "", "root of mount point")
}

func parseFlag() {
	flag.Parse()

	if conf == "" {
		stdlog.Fatal("Specify config file via --conf")
	}

	if path == "" {
		stdlog.Fatal("Specify volume path via --path")
	}
}

func loadConfig(conf string) ([]v1alpha1.IOChaosAction, error) {
	data, err := ioutil.ReadFile(conf)
	if err != nil {
		return nil, err
	}

	actions := []v1alpha1.IOChaosAction{}
	if err := json.Unmarshal(data, &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

type processManger struct {
	bpm *bpm.BackgroundProcessManager
}

func newProcessManger() *processManger {
	logger, err := log.NewDefaultZapLogger()
	if err != nil {
		stdlog.Fatal("Error when init logger:", err)
	}

	logger = logger.WithName("todautil")
	log.ReplaceGlobals(logger)

	return &processManger{
		bpm: bpm.StartBackgroundProcessManager(nil, logger),
	}
}

func (pm *processManger) startProcess(
	ctx context.Context, cmd *bpm.ManagedCommand,
) *bpm.Process {
	proc, err := pm.bpm.StartProcess(ctx, cmd)
	if err != nil {
		stdlog.Fatal("Error when start process:", err)
	}
	return proc
}

func (pm *processManger) killBackgroundProcess(
	ctx context.Context, uid string,
) error {
	return pm.bpm.KillBackgroundProcess(ctx, uid)
}

func main() {
	parseFlag()

	// Load configuration
	actions, err := loadConfig(conf)
	if err != nil {
		stdlog.Fatal("Error when load config:", err)
	}

	if len(actions) == 0 {
		stdlog.Print("no injector")
		os.Exit(0)
	}

	// build command
	args := fmt.Sprintf("--path %s --verbose info --mount-only", path)
	stdlog.Printf("executing cmd: %s %s", todaBin, args)

	processBuilder := bpm.DefaultProcessBuilder(
		todaBin, strings.Split(args, " ")...,
	).EnableLocalMnt().SetIdentifier(fmt.Sprintf("toda-%s", path))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := processBuilder.Build(ctx)
	cmd.Stderr = os.Stderr

	// Start toda
	pm := newProcessManger()
	proc := pm.startProcess(ctx, cmd)

	// Construct jrpc client
	client, err := jrpc.DialIO(ctx, proc.Pipes.Stdout, proc.Pipes.Stdin)
	if err != nil {
		stdlog.Fatal("Error when initialize jrpc client:", err)
	}

	// Set signal notifier
	var exitState int32 = 1
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Enable io faults
	var ret string
	stdlog.Print("Waiting for toda to start")
	var rpcError error
	maxWaitTime := time.Millisecond * 2000
	timeOut, cancel := context.WithTimeout(ctx, maxWaitTime)
	defer cancel()
	_ = client.CallContext(timeOut, &ret, "update", actions)
	rpcError = client.CallContext(timeOut, &ret, "get_status", "ping")
	if rpcError != nil || ret != "ok" {
		if err := pm.killBackgroundProcess(ctx, proc.Uid); err != nil {
			stdlog.Print(err, "kill toda")
		}
		stdlog.Fatalf("toda startup takes too long or an error occurs: %s", ret)
	}

EXIT:
	for {
		sig := <-sc
		stdlog.Printf("Receive signal: [%s]", sig.String())

		switch sig {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			atomic.StoreInt32(&exitState, 0)
			if err := pm.killBackgroundProcess(
				context.Background(), proc.Uid,
			); err != nil {
				stdlog.Print(err, "kill toda")
			}

			break EXIT
		case syscall.SIGHUP:
		default:
			break EXIT
		}
	}
}
