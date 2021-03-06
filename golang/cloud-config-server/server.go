// cloud-config-server starts an HTTP server, which can be accessed
// via URLs in the form of
//
//   http://<addr:port>?mac=aa:bb:cc:dd:ee:ff
//
// and returns the cloud-config YAML file specificially tailored for
// the node whose primary NIC's MAC address matches that specified in
// above URL.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/golang/glog"

	"github.com/gorilla/mux"
	"github.com/k8sp/sextant/golang/certgen"
	cctemplate "github.com/k8sp/sextant/golang/template"
	"github.com/topicai/candy"
)

func main() {
	clusterDesc := flag.String("cluster-desc", "./cluster-desc.yml", "Configurations for a k8s cluster.")
	ccTemplateDir := flag.String("cloud-config-dir", "./cloud-config.template", "cloud-config file template.")
	caCrt := flag.String("ca-crt", "", "CA certificate file, in PEM format")
	caKey := flag.String("ca-key", "", "CA private key file, in PEM format")
	addr := flag.String("addr", ":8080", "Listening address")
	staticDir := flag.String("dir", "./static/", "The directory to serve files from. Default is ./static/")
	flag.Parse()

	if len(*caCrt) == 0 || len(*caKey) == 0 {
		glog.Info("No ca.pem or ca-key.pem provided, generating now...")
		*caKey, *caCrt = certgen.GenerateRootCA("./")
	}
	// valid caKey and caCrt file is ready
	if err := fileExist(*caCrt); err != nil {
		glog.Error("No cert of ca.pem has been generated!")
		log.Panic(err)
	}
	if err := fileExist(*caKey); err != nil {
		glog.Error("No cert of ca-key.pem has been generated!")
		log.Panic(err)
	}

	glog.Info("Cloud-config server start Listenning...")
	l, e := net.Listen("tcp", *addr)
	candy.Must(e)

	// start and run the HTTP server
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/cloud-config/{mac}", makeCloudConfigHandler(*clusterDesc, *ccTemplateDir, *caKey, *caCrt))
	router.HandleFunc("/centos/post-script/{mac}", makeCentOSPostScriptHandler(*clusterDesc, *ccTemplateDir, *caKey, *caCrt))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(*staticDir))))

	glog.Fatal(http.Serve(l, router))
}

// makeCloudConfigHandler generate a HTTP server handler to serve cloud-config
// fetching requests
func makeCloudConfigHandler(clusterDescFile string, ccTemplateDir string, caKey, caCrt string) http.HandlerFunc {
	return makeSafeHandler(func(w http.ResponseWriter, r *http.Request) {
		hwAddr, err := net.ParseMAC(mux.Vars(r)["mac"])
		candy.Must(err)
		candy.Must(cctemplate.Execute(w, hwAddr.String(), "cc-template", ccTemplateDir, clusterDescFile, caKey, caCrt))
	})
}

func makeCentOSPostScriptHandler(clusterDescFile string, ccTemplateDir string, caKey, caCrt string) http.HandlerFunc {
	return makeSafeHandler(func(w http.ResponseWriter, r *http.Request) {
		hwAddr, err := net.ParseMAC(mux.Vars(r)["mac"])
		candy.Must(err)
		candy.Must(cctemplate.Execute(w, hwAddr.String(), "centos-post-script", ccTemplateDir, clusterDescFile, caKey, caCrt))
	})
}

func makeSafeHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			}
		}()
		h(w, r)
	}
}

func fileExist(fn string) error {
	_, err := os.Stat(fn)
	if err != nil || os.IsNotExist(err) {
		return errors.New("file " + fn + " is not ready.")
	}
	return nil
}
