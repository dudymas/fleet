package registry

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	log "github.com/golang/glog"

	"github.com/coreos/fleet/event"
	"github.com/coreos/fleet/job"
	"github.com/coreos/fleet/machine"
)

const (
	offerPrefix = "offer"
)

func (r *Registry) CreateJobOffer(jo *job.JobOffer) {
	key := path.Join(keyPrefix, offerPrefix, jo.Job.Name, "object")
	json, err := marshal(jo)
	if err != nil {
		log.Errorf(err.Error())
		return
	}
	r.etcd.Set(key, json, 0)
}

func (r *Registry) UnresolvedJobOffers() []job.JobOffer {
	var offers []job.JobOffer

	key := path.Join(keyPrefix, offerPrefix)
	resp, err := r.etcd.Get(key, true, true)

	if err != nil {
		return offers
	}

	for _, node := range resp.Node.Nodes {
		key := path.Join(node.Key, "object")
		resp, err := r.etcd.Get(key, true, true)

		var jo job.JobOffer
		err = unmarshal(resp.Node.Value, &jo)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		offers = append(offers, jo)
	}

	return offers
}

func (r *Registry) ClaimJobOffer(jobName string, m *machine.Machine, ttl time.Duration) bool {
	key := path.Join(keyPrefix, offerPrefix, jobName)
	_, err := r.etcd.Get(key, false, true)
	if err != nil {
		return false
	}

	return r.acquireLeadership(fmt.Sprintf("offer-%s", jobName), m.BootId, ttl)
}

func (r *Registry) ResolveJobOffer(jobName string) {
	key := path.Join(keyPrefix, offerPrefix, jobName)
	_, err := r.etcd.Delete(key, true)
	if err == nil {
		log.V(2).Infof("Successfully resolved JobOffer(%s)", jobName)
	} else {
		log.V(2).Infof("Failed to resolve JobOffer(%s): %s", jobName, err.Error())
	}
}

func (r *Registry) SubmitJobBid(jb *job.JobBid) {
	key := path.Join(keyPrefix, offerPrefix, jb.JobName, "bids", jb.MachineName)
	//TODO: Use a TTL
	r.etcd.Set(key, "", 0)
}

func (self *EventStream) filterEventJobOffered(resp *etcd.Response) *event.Event {
	if resp.Action != "set" {
		return nil
	}

	dir, base := path.Split(resp.Node.Key)

	if base != "object" {
		return nil
	}

	dir = path.Dir(strings.TrimSuffix(dir, "/"))
	prefix := path.Base(strings.TrimSuffix(dir, "/"))

	if prefix != offerPrefix {
		return nil
	}

	var jo job.JobOffer
	//TODO: handle error from unmarshal
	unmarshal(resp.Node.Value, &jo)

	return &event.Event{"EventJobOffered", jo, nil}
}

func filterEventJobBidSubmitted(resp *etcd.Response) *event.Event {
	if resp.Action != "set" {
		return nil
	}

	dir, machName := path.Split(resp.Node.Key)
	dir, prefix := path.Split(strings.TrimSuffix(dir, "/"))

	if prefix != "bids" {
		return nil
	}

	dir, jobName := path.Split(strings.TrimSuffix(dir, "/"))
	prefix = path.Base(strings.TrimSuffix(dir, "/"))

	if prefix != offerPrefix {
		return nil
	}

	jb := job.NewBid(jobName, machName)
	return &event.Event{"EventJobBidSubmitted", *jb, nil}
}
