package lan

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/csv"
	"io"
	"net"
	"strings"
	"sync"
)

//go:embed data/vendors.csv.gz
var vendorData embed.FS
var vendorCache = struct {
	sync.Mutex
	names map[string]string
}{names: map[string]string{}}

// Stream the compact offline database only for previously unseen prefixes; the
// tray never holds a full vendor registry or makes Internet vendor queries.
func vendorsFor(devices []Device) map[string]string {
	vendorCache.Lock()
	defer vendorCache.Unlock()
	if len(vendorCache.names) > 4096 {
		vendorCache.names = map[string]string{}
	}
	wanted := map[string]bool{}
	keys := map[string][]string{}
	out := map[string]string{}
	for _, d := range devices {
		mac := validMAC(d.MAC)
		if mac == "" {
			continue
		}
		hardware, _ := net.ParseMAC(mac)
		if hardware[0]&2 != 0 {
			continue
		}
		flat := strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
		prefixes := []string{flat[:9], flat[:7], flat[:6]}
		keys[mac] = prefixes
		for _, p := range prefixes {
			if _, ok := vendorCache.names[p]; !ok {
				wanted[p] = true
			}
		}
	}
	if len(wanted) > 0 {
		raw, _ := vendorData.ReadFile("data/vendors.csv.gz")
		reader, err := gzip.NewReader(bytes.NewReader(raw))
		if err == nil {
			csvReader := csv.NewReader(reader)
			csvReader.ReuseRecord = true
			for {
				row, err := csvReader.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				if len(row) == 2 && wanted[row[0]] {
					vendorCache.names[row[0]] = row[1]
				}
			}
			reader.Close()
		}
		for p := range wanted {
			if _, ok := vendorCache.names[p]; !ok {
				vendorCache.names[p] = ""
			}
		}
	}
	for mac, prefixes := range keys {
		for _, p := range prefixes {
			if name := vendorCache.names[p]; name != "" {
				out[mac] = name
				break
			}
		}
	}
	return out
}
