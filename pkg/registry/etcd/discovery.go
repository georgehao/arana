package etcd

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

import (
	"github.com/pkg/errors"
)

import (
	"github.com/arana-db/arana/pkg/registry"
	"github.com/arana-db/arana/pkg/registry/store"
	"github.com/arana-db/arana/pkg/util/log"
)

// EtcdV3Discovery is a etcd service discovery.
// It always returns the registered servers in etcd.
type EtcdV3Discovery struct {
	BasePath string

	services  []*registry.ServiceInstance
	serviceMu sync.RWMutex

	mu    sync.Mutex
	chans []chan *registry.ServiceInstance

	// -1 means it always retry to watch until zookeeper is ok, 0 means no retry.
	RetriesAfterWatchFailed int

	client store.Store
	stopCh chan struct{}
}

// NewEtcdV3Discovery returns a new EtcdV3Discovery.
func NewEtcdV3Discovery(basePath string, servicePath string, etcdAddrs []string, options *store.Options) (registry.Discovery, error) {
	discoveryPath := basePath + "/" + servicePath
	if len(discoveryPath) > 1 && strings.HasSuffix(discoveryPath, "/") {
		discoveryPath = discoveryPath[:len(discoveryPath)-1]
	}
	etcdV3Discovery := EtcdV3Discovery{
		BasePath:                discoveryPath,
		stopCh:                  make(chan struct{}),
		RetriesAfterWatchFailed: -1,
	}

	store.AddStore(store.ETCD, store.NewEtcdV3)
	client, err := store.NewStore(store.ETCD, etcdAddrs, options)
	if err != nil {
		log.Errorf("EtcdV3 Registry create etcdv3 client err:%v", err)
		return nil, errors.Wrap(err, "EtcdV3 Registry create etcdv3 client")
	}
	etcdV3Discovery.client = client

	ps, err := client.List(context.Background(), discoveryPath)
	if err != nil {
		log.Errorf("cannot get services of from registry: %v, err: %v", discoveryPath, err)
		return nil, err
	}

	for _, val := range ps {
		var tmpService registry.ServiceInstance
		if err := json.Unmarshal(val, &tmpService); err != nil {
			log.Warnf("watchtree unmarshal err:%v", err)
			continue
		}
		etcdV3Discovery.services = append(etcdV3Discovery.services, &tmpService)
	}

	go etcdV3Discovery.watch(context.Background())

	return &etcdV3Discovery, nil
}

// GetServices returns the servers
func (d *EtcdV3Discovery) GetServices() []*registry.ServiceInstance {
	d.serviceMu.RLock()
	defer d.serviceMu.RUnlock()
	return d.services
}

// WatchService returns a nil chan.
func (d *EtcdV3Discovery) WatchService() chan *registry.ServiceInstance {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch := make(chan *registry.ServiceInstance, 10)
	d.chans = append(d.chans, ch)
	return ch
}

func (d *EtcdV3Discovery) RemoveWatcher(ch chan *registry.ServiceInstance) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var chans []chan *registry.ServiceInstance
	for _, c := range d.chans {
		if c == ch {
			continue
		}
		chans = append(chans, c)
	}
	d.chans = chans
}

func (d *EtcdV3Discovery) watch(ctx context.Context) {
	defer func() {
		d.client.Close()
	}()

rewatch:
	for {
		var err error
		var tempDelay time.Duration

		serviceChan := make(chan *registry.ServiceInstance)
		retry := d.RetriesAfterWatchFailed
		for d.RetriesAfterWatchFailed < 0 || retry >= 0 {
			tmpValChan := make(<-chan []byte)
			tmpValChan, err = d.client.WatchTree(ctx, d.BasePath, nil)
			if err != nil {
				if d.RetriesAfterWatchFailed > 0 {
					retry--
				}
				if tempDelay == 0 {
					tempDelay = 1 * time.Second
				} else {
					tempDelay *= 2
				}
				if max := 30 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Warnf("can not watchtree (with retry %d, sleep %v): %s: %v", retry, tempDelay, d.BasePath, err)
				time.Sleep(tempDelay)
				continue
			}

			var tmpService registry.ServiceInstance
			if err := json.Unmarshal(<-tmpValChan, &tmpService); err != nil {
				log.Warnf("watchtree unmarshal err:%v", err)
				continue
			}
			serviceChan <- &tmpService
			break
		}

		if err != nil {
			log.Errorf("can't watch %s: %v", d.BasePath, err)
			return
		}

		for {
			select {
			case <-d.stopCh:
				log.Info("discovery has been closed")
				return
			case service, ok := <-serviceChan:
				if !ok {
					break rewatch
				}
				if service == nil {
					continue
				}

				d.serviceMu.Lock()
				d.services = append(d.services, service)
				d.serviceMu.Unlock()

				d.mu.Lock()
				for _, ch := range d.chans {
					ch := ch
					go func() {
						defer func() {
							recover()
						}()

						select {
						case ch <- service:
						case <-time.After(time.Minute):
							log.Warn("chan is full and new change has been dropped")
						}
					}()
				}
				d.mu.Unlock()
			}
		}
	}
}

func (d *EtcdV3Discovery) Close() {
	close(d.stopCh)
}
