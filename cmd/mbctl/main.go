package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cilium/ebpf"
	v1 "k8s.io/api/core/v1"

	"github.com/merbridge/merbridge/internal/ebpfs"
	"github.com/merbridge/merbridge/internal/pods"
	"github.com/merbridge/merbridge/pkg/kube"
	"github.com/merbridge/merbridge/pkg/linux"
)

var currentNodeIP string // for cache

func main() {
	mode := ""
	debug := false
	flag.StringVar(&mode, "m", "istio", "Service mesh mode, current support istio and linkerd")
	flag.BoolVar(&debug, "d", false, "Debug mode")
	flag.Parse()
	if mode != "istio" && mode != "linkerd" {
		fmt.Printf("unsupport mode %q, current support istio and linkerd\n", mode)
		os.Exit(1)
	}
	if err := ebpfs.LoadMBProgs(mode, debug); err != nil {
		panic(err)
	}
	m, err := ebpf.LoadPinnedMap("/sys/fs/bpf/local_pod_ips", &ebpf.LoadPinOptions{})
	if err != nil {
		fmt.Printf("load map error: %v", err)
		os.Exit(1)
	}
	cli, err := kube.GetKubernetesClientWithFile("", "")
	if err != nil {
		panic(err)
	}
	locaName, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	addFunc := func(obj interface{}) {
		if pod, ok := obj.(*v1.Pod); ok {
			if !pods.IsInjectedSidecar(pod) {
				return
			}
			fmt.Printf("got pod updated %s/%s\n", pod.Namespace, pod.Name)
			podHostIP := pod.Status.HostIP
			if currentNodeIP == "" {
				if linux.IsCurrentNodeIP(podHostIP) {
					currentNodeIP = podHostIP
				}
			}
			if podHostIP == currentNodeIP {
				_ip, _ := linux.IP2Linux(pod.Status.PodIP)
				err := m.Update(_ip, uint32(0), ebpf.UpdateAny)
				if err != nil {
					fmt.Printf("update process ip %s error: %v", pod.Status.PodIP, err)
				}
			}
		}
	}

	updateFunc := func(old, new interface{}) {
		addFunc(new)
	}
	deleteFunc := func(obj interface{}) {
		if pod, ok := obj.(*v1.Pod); ok {
			fmt.Printf("got pod delete %s/%s\n", pod.Namespace, pod.Name)
			_ip, _ := linux.IP2Linux(pod.Status.PodIP)
			_ = m.Delete(_ip)
		}
	}
	w := pods.NewWatcher(cli, locaName, addFunc, updateFunc, deleteFunc)
	_ = w.Start()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
	<-ch
	w.Stop()
	_ = ebpfs.UnLoadMBProgs()
}
