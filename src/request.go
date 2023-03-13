package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/influxdata/influxdb1-client"
	influx "github.com/influxdata/influxdb1-client/v2"
	maxminddb "github.com/oschwald/maxminddb-golang"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// getJSONByUrl requests data from the given URL, authenticates with basic auth
// and decodes the (mandatory) JSON result into target.
func getJSONByUrl(url string, accessKey string, secretKey string, insecure bool, target interface{}) error {
	start := time.Now()

	log.Info("Connecting to ", url)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return fmt.Errorf("stopped after 4 redirects")
			}
			req.SetBasicAuth(accessKey, secretKey)
			return nil
		},
	}

	if insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		}
		client.Transport = tr
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error("Error creating GET request to collect JSON from API")
		panic(err)
	}
	req.SetBasicAuth(accessKey, secretKey)

	resp, err := client.Do(req)
	if err != nil {
		log.Error("Error Collecting JSON from API: ", err)
		panic(err)
	}

	respFormatted := json.NewDecoder(resp.Body).Decode(target)

	// Timings recorded as part of internal metrics
	log.Info(
		"Time to fetch JSON from URL: ",
		float64((time.Since(start))/time.Millisecond),
		"ms ",
		url,
	)

	// Close the response body, the underlying Transport should then close the connection.
	resp.Body.Close()

	// return formatted JSON
	return respFormatted
}

// getJSONByFile reads the content of a file and unmarshalls it to the target
// variable.
func getJSONByFile(name string, target interface{}) error {
	byteValue, _ := ioutil.ReadFile(name)
	return json.Unmarshal(byteValue, target)
}

type reqLocation struct {
	City    string `json:"city"`
	Country struct {
		Name    string
		ISOCode string
	} `json:"country"`
}

type Request struct {
	Id        int64                  `json:"id"`
	Uid       string                 `json:"uid"`
	FirstSeen time.Time              `json:"first_seen"`
	LastSeen  time.Time              `json:"last_seen"`
	LastIp    string                 `json:"last_ip"`
	RecordVer string                 `json:"record_version"`
	Record    map[string]interface{} `json:"record"`
	Location  reqLocation            `json:"location"`
	Status    string                 `json:"status"`
}

func newrequestByString(line, sep string) *Request {
	var err error

	s := strings.Split(line, sep)
	if len(s) == 6 {
		req := &Request{}

		req.Id, err = strconv.ParseInt(s[0], 10, 64)
		if err != nil {
			log.Info("Error parsing Id ", s[0], err)
		}
		req.Uid = s[1]
		req.FirstSeen, err = time.Parse("2006-01-02 15:04:05.999999", s[2])
		if err != nil {
			log.Info("Error parsing Firstseen ", s[2], err)
		}
		req.LastSeen, err = time.Parse("2006-01-02 15:04:05.999999", s[3])
		if err != nil {
			log.Info("Error parsing Lastseen ", s[3], err)
		}
		req.LastIp = s[4]
		err = json.Unmarshal([]byte(s[5]), &req.Record)
		if err != nil {
			log.Info("Error unmarshalling Record ", s[5], err)
		}

		return req
	}
	return nil
}

// checkData tests if the record exists and whether certain fields are present
// in the record. Depending on the record version, different fields are checked
// for their existence.  It returns false if a key does not exist.  It only
// returns true if all keys exist.
//
// It also populates the r.RecordVer field.
func (r *Request) checkData() bool {
	if len(r.Uid) < 3 || r.Record == nil {
		return false
	}

	r.RecordVer = strconv.FormatFloat(r.Record["r"].(float64), 'f', 0, 64)

	var record_key []string

	if r.RecordVer == "1" {
		record_key = []string{"container", "environment", "host", "service", "stack", "install"}
	}

	if r.RecordVer == "2" {
		record_key = []string{"cluster", "node", "project", "install"}
	}

	for _, key := range record_key {
		if r.Record[key] == nil {
			return false
		}
	}

	return true
}

// Produce wire-formatted string for ingestion into influxdb
func (r *Request) printInflux() {
	for _, p := range r.getPoints() {
		fmt.Println(p.String())
	}
}

// getPoints collects the various InfluxDB points of the categories apps,
// drivers and telemetry from the record previously obtained and returns them.
func (r *Request) getPoints() []*influx.Point {
	points := append(r.getTelemetryAppPoints(), r.getTelemetryDriverPoints()...)
	return append(points, r.getTelemetryPoint())
}

// getTelemetryAppPoints collects the app points from the current record,
// converts them to influx.Point and returns them. If the version of the record
// is not 2, it will return an empty slice.
func (r *Request) getTelemetryAppPoints() []*influx.Point {
	// out := []*influx.Point{}
	var out []*influx.Point
	if r.RecordVer != "2" {
		return out
	}

	name := "telemetry_apps"
	tags := map[string]string{
		"id":  strconv.FormatInt(r.Id, 10),
		"uid": r.Uid,
	}
	fields := map[string]interface{}{
		"uid": r.Uid,
	}
	if apps, ok := r.Record["app"].(map[string]interface{}); ok {
		if catalogs, ok := apps["rancheCatalogs"].(map[string]interface{}); ok {
			for catalogKey := range catalogs {
				if appList, ok := catalogs[catalogKey].(map[string]interface{})["apps"].(map[string]interface{}); ok {
					for appKey := range appList {
						if appVer, ok := appList[appKey].(map[string]interface{}); ok {
							for verKey := range appVer {
								tags["catalog"] = catalogKey
								tags["name"] = appKey
								tags["version"] = verKey
								fields["total"] = appVer[verKey]
								p, err := influx.NewPoint(name, tags, fields, r.LastSeen)
								if err != nil {
									log.Warn(err)
									continue
								}
								out = append(out, p)
							}
						}
					}
				}
			}
		}
	}

	return out
}

// getTelemetryDriverPoints collects the driver points from the current record,
// converts them to influx.Point and returns them. If the version of the record
// is not 2, it will return an empty slice.
func (r *Request) getTelemetryDriverPoints() []*influx.Point {
	out := []*influx.Point{}
	if r.RecordVer != "2" {
		return out
	}

	name := "telemetry_drivers"
	tags := map[string]string{
		"id":  strconv.FormatInt(r.Id, 10),
		"uid": r.Uid,
	}
	fields := map[string]interface{}{
		"uid": r.Uid,
	}
	if clusterDrivers, ok := r.Record["cluster"].(map[string]interface{})["driver"].(map[string]interface{}); ok {
		for driverKey := range clusterDrivers {
			tags["kind"] = "cluster"
			tags["name"] = driverKey
			fields["total"] = clusterDrivers[driverKey]
			p, err := influx.NewPoint(name, tags, fields, r.LastSeen)
			if err != nil {
				log.Warn(err)
				continue
			}
			out = append(out, p)
		}
	}
	if nodeDrivers, ok := r.Record["node"].(map[string]interface{})["driver"].(map[string]interface{}); ok {
		for driverKey := range nodeDrivers {
			tags["kind"] = "node"
			tags["name"] = driverKey
			fields["total"] = nodeDrivers[driverKey]
			m, err := influx.NewPoint(name, tags, fields, r.LastSeen)
			if err != nil {
				log.Warn(err)
				continue
			}
			out = append(out, m)
		}
	}

	return out
}

func (r *Request) getTelemetryPoint() *influx.Point {
	var name = "telemetry"
	var fields map[string]interface{}
	var tags map[string]string

	if r.RecordVer == "1" {
		orch := r.Record["environment"].(map[string]interface{})["orch"].(map[string]interface{})
		orch_key := [5]string{"cattle", "kubernetes", "mesos", "swarm", "windows"}
		for _, key := range orch_key {
			if orch[key] == nil {
				orch[key] = float64(0)
			}
		}
		fields = map[string]interface{}{
			"ip":                   r.LastIp,
			"uid":                  r.Uid,
			"container_running":    r.Record["container"].(map[string]interface{})["running"],
			"container_total":      r.Record["container"].(map[string]interface{})["total"],
			"environment_total":    r.Record["environment"].(map[string]interface{})["total"],
			"orch_cattle":          orch["cattle"],
			"orch_kubernetes":      orch["kubernetes"],
			"orch_mesos":           orch["mesos"],
			"orch_swarm":           orch["swarm"],
			"orch_windows":         orch["windows"],
			"host_active":          r.Record["host"].(map[string]interface{})["active"],
			"host_cpu_cores_total": r.Record["host"].(map[string]interface{})["cpu"].(map[string]interface{})["cores_total"],
			"host_mem_mb_total":    r.Record["host"].(map[string]interface{})["mem"].(map[string]interface{})["mb_total"],
			"service_active":       r.Record["service"].(map[string]interface{})["active"],
			"service_total":        r.Record["service"].(map[string]interface{})["total"],
			"stack_active":         r.Record["stack"].(map[string]interface{})["active"],
			"stack_total":          r.Record["stack"].(map[string]interface{})["total"],
			"stack_from_catalog":   r.Record["stack"].(map[string]interface{})["from_catalog"],
		}
	}
	if r.RecordVer == "2" {
		cluster := r.Record["cluster"].(map[string]interface{})
		node := r.Record["node"].(map[string]interface{})
		project := r.Record["project"].(map[string]interface{})
		fields = map[string]interface{}{
			"ip":                             r.LastIp,
			"uid":                            r.Uid,
			"cluster_active":                 cluster["active"],
			"cluster_total":                  cluster["total"],
			"cluster_namespace_total":        cluster["namespace"].(map[string]interface{})["total"],
			"cluster_namespace_from_catalog": cluster["namespace"].(map[string]interface{})["from_catalog"],
			"cluster_cpu_total":              cluster["cpu"].(map[string]interface{})["cores_total"],
			"cluster_cpu_util":               cluster["cpu"].(map[string]interface{})["util_avg"],
			"cluster_driver_aks":             cluster["driver"].(map[string]interface{})["azureKubernetesService"],
			"cluster_driver_eks":             cluster["driver"].(map[string]interface{})["amazonElasticContainerService"],
			"cluster_driver_gke":             cluster["driver"].(map[string]interface{})["googleKubernetesEngine"],
			"cluster_driver_imported":        cluster["driver"].(map[string]interface{})["imported"],
			"cluster_driver_rke":             cluster["driver"].(map[string]interface{})["rancherKubernetesEngine"],
			"cluster_driver_k3s":             cluster["driver"].(map[string]interface{})["k3s"],
			"cluster_driver_k3sBased":        cluster["driver"].(map[string]interface{})["k3sBased"],
			"cluster_mem_mb_total":           cluster["mem"].(map[string]interface{})["mb_total"],
			"cluster_mem_util":               cluster["mem"].(map[string]interface{})["util_avg"],
			"node_active":                    node["active"],
			"node_total":                     node["total"],
			"node_driver_azure":              node["driver"].(map[string]interface{})["azure"],
			"node_driver_ec2":                node["driver"].(map[string]interface{})["amazonec2"],
			"node_driver_do":                 node["driver"].(map[string]interface{})["digitalocean"],
			"node_driver_openstack":          node["driver"].(map[string]interface{})["openstack"],
			"node_driver_vsphere":            node["driver"].(map[string]interface{})["vmwarevsphere"],
			"node_from_template":             node["from_template"],
			"node_imported":                  node["imported"],
			"node_mem_mb_total":              node["mem"].(map[string]interface{})["mb_total"],
			"node_mem_util":                  node["mem"].(map[string]interface{})["util_avg"],
			"node_role_controlplane":         node["role"].(map[string]interface{})["controlplane"],
			"node_role_etcd":                 node["role"].(map[string]interface{})["etcd"],
			"node_role_worker":               node["role"].(map[string]interface{})["worker"],
			"project_total":                  project["total"],
			"project_namespace_total":        project["namespace"].(map[string]interface{})["total"],
			"project_namespace_from_catalog": project["namespace"].(map[string]interface{})["from_catalog"],
			"project_workload_total":         project["workload"].(map[string]interface{})["total"],
			"project_pod_total":              project["pod"].(map[string]interface{})["total"],
		}

		if value, ok := cluster["istio"]; ok {
			fields["cluster_istio_total"] = value
		}
		if value, ok := cluster["monitoring"]; ok {
			fields["cluster_monitoring_total"] = value
		}

		if logProvider, ok := cluster["logging"].(map[string]interface{}); ok {
			for key := range logProvider {
				lowerKey := strings.ToLower(key)
				if value, ok := logProvider[key].(float64); ok {
					fields["cluster_logging_provider_"+lowerKey] = int(value)
				} else {
					fields["cluster_logging_provider_"+lowerKey] = 0
				}
				fields["cluster_logging_provider_"+lowerKey] = logProvider[key]
			}
		}

		if cloudProvider, ok := cluster["cloudProvider"].(map[string]interface{}); ok {
			for key := range cloudProvider {
				if value, ok := cloudProvider[key].(float64); ok {
					if key == "gce" || key == "external" {
						fields["cluster_cloud_provider_"+key] = value
						continue
					}
					fields["cluster_cloud_provider_"+key] = int(value)
				} else {
					if key == "gce" || key == "external" {
						fields["cluster_cloud_provider_"+key] = float64(0)
						continue
					}
					fields["cluster_cloud_provider_"+key] = 0
				}
			}
		}
	}

	installImage := "rancher/rancher"
	if image := r.Record["install"].(map[string]interface{})["image"]; image != nil {
		installImage = image.(string)
	}

	tags = map[string]string{
		"id":              strconv.FormatInt(r.Id, 10),
		"uid":             r.Uid,
		"ip":              r.LastIp,
		"install_image":   installImage,
		"install_version": r.Record["install"].(map[string]interface{})["version"].(string),
		"record_version":  r.RecordVer,
		"city":            r.Location.City,
		"country":         r.Location.Country.Name,
		"country_isocode": r.Location.Country.ISOCode,
		"status":          r.Status,
	}

	p, err := influx.NewPoint(name, tags, fields, r.LastSeen)
	if err != nil {
		log.Warn(err)
	}

	return p
}

func (r *Request) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

func (r *Request) printJson() {

	j, err := json.Marshal(r)
	if err != nil {
		log.Error("json", err)
	}

	fmt.Println(string(j))

}

func (r *Request) getLocation(geoipdb string) {
	db, err := maxminddb.Open(geoipdb)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ip := net.ParseIP(r.LastIp)

	var record struct {
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Country struct {
			Names   map[string]string `maxminddb:"names"`
			ISOCode string            `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	} // Or any appropriate struct

	err = db.Lookup(ip, &record)
	if err != nil {
		log.Fatal(err)
	}

	r.Location.City = record.City.Names["en"]
	r.Location.Country.Name = record.Country.Names["en"]
	r.Location.Country.ISOCode = record.Country.ISOCode
}

func (r *Request) isNew() string {
	diff := r.LastSeen.Sub(r.FirstSeen)

	if diff.Hours() > 24 {
		return "active"
	} else {
		return "new"
		// TODO look into the status new. If any new clusters have sent a report
		// while telemetry stats was absent, setting the first few records
		// received to the status "new" will only work if r.FirstSeen is set
		// correctly!!! Which it currently isn't. Instead, it uses its zero
		// value.
	}
}

// getData checks the validity of the record using `Request.checkData()` and
// retrieves and fills the location of the IP address in the record using a
// location database.  It also updates the status of the record (based on the
// time of the record). Returns false if any of those operations failed.
func (r *Request) getData(geoipdb string) bool {
	if !r.checkData() {
		log.Debugf("Skipping request without correct data")
		return false
	}

	now := time.Now()
	diff := now.Sub(r.LastSeen)

	if diff < 0 {
		log.Debugf("Skipping request with time in future %v", r.LastSeen)
		r.printJson()
		return false
	}

	r.getLocation(geoipdb)
	r.Status = r.isNew()

	return true

}

// Params are the parameters necessary to create a Requests object.
type Params struct {
	url          string
	hours        int
	accessKey    string
	secretKey    string
	influxurl    string
	influxdb     string
	influxuser   string
	influxpass   string
	insecure     bool
	geoipdb      string
	file         string
	format       string
	preview      bool
	limit        int
	refresh      int
	flush        int
	restoreDates Dates
}

func RunRequests(c *cli.Context) error {
	restoreDates, err := NewDates(c.String("dates"))
	if err != nil {
		return err
	}
	params := Params{
		url:          c.String("url"),
		hours:        c.Int("hours"),
		accessKey:    c.String("accesskey"),
		secretKey:    c.String("secretkey"),
		influxurl:    c.String("influxurl"),
		influxdb:     c.String("influxdb"),
		influxuser:   c.String("influxuser"),
		influxpass:   c.String("influxpass"),
		insecure:     c.Bool("insecure"),
		geoipdb:      c.String("geoipdb"),
		file:         c.String("file"),
		format:       c.String("format"),
		preview:      c.Bool("preview"),
		limit:        c.Int("limit"),
		refresh:      c.Int("refresh"),
		flush:        c.Int("flush"),
		restoreDates: restoreDates,
	}
	req := newRequests(params)
	req.getDataByUrl()

	return nil
}

// Requests is the main struct where data is being fetched from one source and
// put into another.
type Requests struct {
	Input   chan *Request
	Output  chan *influx.Point
	Exit    chan os.Signal
	Readers []chan struct{}
	Config  Params
	Data    chan []Request
}

func newRequests(conf Params) *Requests {
	var r = &Requests{
		Readers: []chan struct{}{},
		Config:  conf,
	}

	r.Input = make(chan *Request, 1)
	r.Output = make(chan *influx.Point, 1)
	r.Exit = make(chan os.Signal, 1)
	r.Data = make(chan []Request, 1)
	signal.Notify(r.Exit, os.Interrupt, syscall.SIGTERM)

	customFormatter := new(log.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	log.SetFormatter(customFormatter)
	customFormatter.FullTimestamp = true

	return r
}

func (r *Requests) Close() {
	close(r.Input)
	close(r.Output)
	close(r.Exit)

}

func (r *Requests) deleteForDay(day string) bool {
	i := newInflux(r.Config.influxurl, r.Config.influxdb, r.Config.influxuser, r.Config.influxpass)
	if i.Init() {
		defer i.Close()
		log.Infof("Deleting from measurement telemetry for day %s", day)
		return i.deleteForDay(day)
	}

	return false
}

func (r *Requests) sendToInflux() {
	var points []influx.Point
	var pointsLength int

	i := newInflux(r.Config.influxurl, r.Config.influxdb, r.Config.influxuser, r.Config.influxpass)

	if i.Init() {
		connected := i.CheckConnect(r.Config.refresh)
		defer i.Close()

		ticker := time.NewTicker(time.Second * time.Duration(r.Config.flush))
		defer ticker.Stop()

		for {
			select {
			case <-connected: // If data was sent to `connected`, that's a quit signal.
				return
			case <-ticker.C: // Try to send the accumulated points we have.
				if len(points) > 0 {
					log.Info("Tick: Sending to influx ", len(points), " points")
					if i.sendToInflux(points, 1) {
						points = []influx.Point{}
					} else {
						return
					}
				}
			case p := <-r.Output: // Read a point from r.Output and add it to `points`.
				if p != nil {
					points = append(points, *p)
					pointsLength = len(points)

					// Send to influx if buffer limit is reached.
					if pointsLength == r.Config.limit {
						log.Info("Batch: Sending to influx ", pointsLength, " points")
						if i.sendToInflux(points, 1) {
							points = []influx.Point{}
						} else {
							log.Warn("Batch: Sending points failed")
							return // If sending fails, exit the function.
						}
					}
				} else {
					// p == nil! Send buffered points to Influx and quit.
					pointsLength = len(points)
					if pointsLength > 0 {
						log.Info("Batch: Sending to influx ", pointsLength, " points")
						if !i.sendToInflux(points, 1) { // Send all points.
							log.Warn("Batch: Sending remaining points failed")
						}
					}
					return // Exit the sendToInflux function.
				}
			}
		}
	}
}

func (r *Requests) addReader() chan struct{} {
	newChannel := make(chan struct{}, 1)
	r.Readers = append(r.Readers, newChannel)

	return newChannel
}

func (r *Requests) closeReaders() {
	for _, readerChannel := range r.Readers {
		if readerChannel != nil {
			readerChannel <- struct{}{}
		}
	}
	r.Readers = nil
}

// getDataByUrl creates a few channels and starts reading data in a goroutine
// and writing data in another goroutine.
func (r *Requests) getDataByUrl() {
	var in, out sync.WaitGroup
	indone := make(chan struct{}, 1)
	outdone := make(chan struct{}, 1)

	in.Add(1)
	go func() {
		defer in.Done()
		// The data is read here.
		r.getDataByChan(r.addReader())
	}()

	out.Add(1)
	go func() {
		defer out.Done()
		// The data is written here.
		r.getOutput()
	}()

	go func() {
		in.Wait()
		close(r.Input)
		close(r.Output)
		close(indone)
	}()

	go func() {
		out.Wait()
		close(outdone)
	}()

	for {
		select {
		case <-indone:
			<-outdone
			return
		case <-outdone:
			log.Error("Aborting...")
			go r.closeReaders()
			return
		case <-r.Exit:
			//close(r.Exit)
			log.Info("Exit signal detected....Closing...")
			go r.closeReaders()
			<-outdone
			return
		}
	}
}

func (r *Requests) getDataByChan(stop chan struct{}) {
	var ticker *time.Ticker

	if len(r.Config.restoreDates) == 0 {
		ticker = time.NewTicker(time.Second * time.Duration(r.Config.refresh))
		defer ticker.Stop()
	}

	r.getData(stop)

	if len(r.Config.restoreDates) == 0 {
		for {
			select {
			case <-ticker.C:
				log.Info("Tick: Getting data...")
				r.getData(stop)
			case <-stop:
				return
			}
		}
	}
}

func (r *Requests) getDataByFile() {
	log.Info("Reading file ", r.Config.file)
	dat, err := ioutil.ReadFile(r.Config.file)

	if err != nil {
		log.Error("Error reading file ", r.Config.file, err)
	}

	requests := <-r.Data

	lines := bytes.Split(dat, []byte("\n"))

	for _, line := range lines {
		if line != nil {
			req := newrequestByString(string(line), "|")
			if req != nil {
				requests = append(requests, *req)
			}
		}
	}

	for _, req := range requests {
		if req.getData(r.Config.geoipdb) {
			if r.Config.format == "json" {
				r.Input <- &req
			} else {
				for _, point := range req.getPoints() {
					r.Output <- point
				}
			}
		}
	}
}

// getJSON returns the active records. A record of a cluster is being treated as
// active if the time of its creation is less than `hours` ago.  Depending on
// the configuration, those records are either read from a file or fetched
// remotely using the configured URL. The data of the record is stored in the
// Requests struct.
func (r *Requests) getJSON() error {
	if r.Config.file != "" {
		err := getJSONByFile(r.Config.file, r)
		if err != nil {
			return fmt.Errorf("error getting JSON from file %s: %v", r.Config.file, err)
		}
		return nil
	}

	var response Response

	path := "/admin/active?hours=" + strconv.Itoa(r.Config.hours)
	err := getJSONByUrl(
		r.Config.url+path,
		r.Config.accessKey,
		r.Config.secretKey,
		r.Config.insecure,
		&response,
	)
	if err != nil {
		return fmt.Errorf("error getting JSON from URL %s: %v", r.Config.url+path, err)
	}

	r.Data <- response.Data

	return nil
}

type Response struct {
	Data []Request `json:"data"`
}

func (r *Requests) getHistoryJSON() error {
	if r.Config.file != "" {
		err := getJSONByFile(r.Config.file, r)
		if err != nil {
			return fmt.Errorf("error getting history JSON from file %s: %v", r.Config.file, err)
		}
		return nil
	}

	go func() {
		defer close(r.Data)
		for _, date := range r.Config.restoreDates {
			var response Response
			path := "/admin/restore/" + date.String()
			log.Debugf("fetching data from path %s", path)
			err := getJSONByUrl(
				r.Config.url+path,
				r.Config.accessKey,
				r.Config.secretKey,
				r.Config.insecure,
				&response,
			)
			if err != nil {
				log.Fatalf("error getting JSON from URL %s: %v", r.Config.url+path, err)
				return
			}
			r.Data <- response.Data
		}
	}()

	return nil
}

// getData fetches the remote data and writes it to the r.Output channel of
// Requests.
func (r *Requests) getData(stop chan struct{}) {
	restoring := len(r.Config.restoreDates) > 0

	// Fetch data and fill r.Data with it.
	var err error
	if !restoring {
		err = r.getJSON()
	} else {
		err = r.getHistoryJSON()
	}
	if err != nil {
		log.Error("Error getting data ", err)
	}

	if restoring {
		// Remove the data to be restored, because we cannot overwrite it, as we
		// don't have the exact time of those points any more (the time is used
		// from installation table, not record table). Installation gets a
		// timestamp a short while after the record has been received, it is not
		// being taken from the record. As installation only holds a timestamp
		// for the (first and) last record, previous timestamps for the last
		// record are removed and lost forever.
		for _, day := range r.Config.restoreDates {
			if !r.deleteForDay(day.String()) {
				stop <- struct{}{}
				return
			}
		}
	}

	for reqs := range r.Data {
		for _, req := range reqs {
			if req.getData(r.Config.geoipdb) { // Validate r.Data and complement r.Location and r.Status
				if r.Config.format == "json" {
					r.Input <- &req
				} else { // influx, the default
					for _, point := range req.getPoints() {
						r.Output <- point
					}
				}
			}
		}
	}

	if restoring {
		// Signal we're done with reading input. Only signal if data is being
		// restored, never if run in "daemon" mode.
		stop <- struct{}{}
	}
}

func (r *Requests) print() {
	if r.Config.format == "json" {
		r.printJson()
	} else {
		r.printInflux()
	}
}

// getOutput writes the data that has been retrieved in parallel from another
// goroutine to InfluxDB. The data is printed if `preview` is enabled.
func (r *Requests) getOutput() {
	if r.Config.format == "json" {
		r.printJson()
	} else {
		if r.Config.preview {
			r.printInflux()
		} else {
			r.sendToInflux()
		}
	}
}

func (r *Requests) printJson() {
	for {
		select {
		case req := <-r.Input:
			if req != nil {
				req.printJson()
			} else {
				return
			}
		}
	}
}

func (r *Requests) printInflux() {
	for {
		select {
		case p := <-r.Output:
			if p != nil {
				fmt.Println(p.String())
			} else {
				return
			}
		}
	}
}
