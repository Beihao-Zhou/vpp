package main

import (
    . "fd.io/hs-test/infra"
    "fmt"
    "os"
    "os/exec"

    . "github.com/onsi/ginkgo/v2"
)

func init() {
    RegisterVethTests(MemcachedTest)
}

func MemcachedTest(s *VethsSuite) {
    var clnVclConf, srvVclConf Stanza
    var ldpreload string

    serverContainer := s.GetContainerByName("server-vpp")
    serverVclFileName := serverContainer.GetHostWorkDir() + "/vcl_srv.conf"

    clientContainer := s.GetContainerByName("client-vpp")
    clientVclFileName := clientContainer.GetHostWorkDir() + "/vcl_cln.conf"

    if *IsDebugBuild {
        ldpreload = "LD_PRELOAD=../../build-root/build-vpp_debug-native/vpp/lib/x86_64-linux-gnu/libvcl_ldpreload.so"
    } else {
        ldpreload = "LD_PRELOAD=../../build-root/build-vpp-native/vpp/lib/x86_64-linux-gnu/libvcl_ldpreload.so"
    }

    stopServerCh := make(chan struct{}, 1)
    srvCh := make(chan error, 1)
    clnCh := make(chan error)
    clnRes := make(chan string, 1)

    s.Log("starting VPPs")

    // Configuration for VCL for client and server
    clientAppSocketApi := fmt.Sprintf("app-socket-api %s/var/run/app_ns_sockets/default",
        clientContainer.GetHostWorkDir())
    err := clnVclConf.
        NewStanza("vcl").
        Append("rx-fifo-size 4000000").
        Append("tx-fifo-size 4000000").
        Append("app-scope-local").
        Append("app-scope-global").
        Append("use-mq-eventfd").
        Append(clientAppSocketApi).Close().
        SaveToFile(clientVclFileName)
    s.AssertNil(err, fmt.Sprint(err))

    serverAppSocketApi := fmt.Sprintf("app-socket-api %s/var/run/app_ns_sockets/default",
        serverContainer.GetHostWorkDir())
    err = srvVclConf.
        NewStanza("vcl").
        Append("rx-fifo-size 4000000").
        Append("tx-fifo-size 4000000").
        Append("app-scope-local").
        Append("app-scope-global").
        Append("use-mq-eventfd").
        Append(serverAppSocketApi).Close().
        SaveToFile(serverVclFileName)
    s.AssertNil(err, fmt.Sprint(err))

    s.Log("attaching server to vpp")

    // Start Memcached Server
    srvEnv := append(os.Environ(), ldpreload, "VCL_CONFIG="+serverVclFileName)
    go func() {
        defer GinkgoRecover()
        StartMemcachedServerApp(s, srvCh, stopServerCh, srvEnv)
    }()

    err = <-srvCh
    s.AssertNil(err, fmt.Sprint(err))

    s.Log("attaching client to vpp")

    // Start Mutilate Client
    clnEnv := append(os.Environ(), ldpreload, "VCL_CONFIG="+clientVclFileName)
    serverVethAddress := s.GetInterfaceByName(ServerInterfaceName).Ip4AddressString()
    go func() {
        defer GinkgoRecover()
        StartMutilateClientApp(s, serverVethAddress, clnEnv, clnCh, clnRes)
    }()
    s.Log(<-clnRes)

    // wait for client's result
    err = <-clnCh
    s.AssertNil(err, fmt.Sprint(err))

    // stop server
    stopServerCh <- struct{}{}
}


func StartMemcachedServerApp(s *VethsSuite, srvCh chan error, stopServerCh chan struct{}, srvEnv []string) {
    s.Log("starting Memcached server")

    // Check if memcached is available
    memcachedPath, err := exec.LookPath("memcached")
    if err != nil {
        srvCh <- fmt.Errorf("memcached not found: %v", err)
        return
    }

    // Log the path of memcached
    s.Log(fmt.Sprintf("memcached found at: %s", memcachedPath))
    go func() {
        cmd := exec.Command(memcachedPath, "-m", "64", "-p", "11211", "-u", "nobody")
        cmd.Env = srvEnv
        s.Log(cmd)
        err := cmd.Start()
        if err != nil {
            srvCh <- fmt.Errorf("failed to start memcached server: %v", err)
        } else {
            srvCh <- nil
        }
        <-stopServerCh
        cmd.Process.Kill()
    }()
}

func StartMutilateClientApp(s *VethsSuite, serverVethAddress string, clnEnv []string, clnCh chan error, clnRes chan string) {
    s.Log("starting Mutilate client")

    // TODO: Fix timeout error
    mutilatePath, err := exec.LookPath("mutilate")
    if err != nil {
        clnCh <- fmt.Errorf("mutilate not found: %v", err)
        return
    }

    s.Log(fmt.Sprintf("mutilate found at: %s", mutilatePath))
    cmd := exec.Command(mutilatePath, "-s", serverVethAddress+":11211")
    cmd.Env = clnEnv
    output, err := cmd.CombinedOutput()
    clnRes <- string(output)
    if err != nil {
        clnCh <- fmt.Errorf("failed to start Mutilate client: %v - output: %s", err, output)
    } else {
        clnCh <- nil
    }
}
