package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
)

var (
	listen_addr   string
	graphite_addr string
	goofy_units   bool
	verbose       bool
)

type TempestMsg struct {
	Serial_number string
	Type          string
	Hub_sn        string
	/* there is really no need for float64 except that they stick timestamp
	   into the same Ob array, which is annoying */
	Evt           []float64
	Ob            []float64
	Obs           [][]float64
	Timestamp     int64
	Uptime        int
	Voltage       float64
	Rssi          int
	Hub_rssi      int
	Sensor_status int
	Debug         int
	Reset_flags   string
	Seq           int
	Fs            []int
	Radio_stats   []int
	Mqtt_stats    []int
	/* The tempest packets have a dumb mistake where hub_status reports
	   firmware_revision as a string, but device_status reports
	   firmware_revision as an int.  Go json parser will not translate
	   either type to the other, so one type of message fails to parse.
	   We ignore the issue by discarding firmware_revision. */
	// Firmware_revision string
}

func graphiteMsg(field string, val float64, timestamp int64) string {
	return fmt.Sprintf("wx.tempest.%s %0.6f %d", field, val, timestamp)
}

func decodeMsg(tm *TempestMsg) []string {

	var gms []string // graphite messages
	// see https://apidocs.tempestwx.com/reference/tempest-udp-broadcast

	switch tm.Type {
	default:
		fmt.Printf("message type %s not decoded\n", tm.Type)
		fmt.Println(tm)
	/*
		case "evt_precip":
			// rain start event
		case "evt_strike":
			// lightning strike event
		case "obs_air":
			// air sensor observation; maybe never sent by tempest system
		case "obs_sky":
			// sky sensor observation; maybe never sent by tempest system
	*/
	case "rapid_wind":
		// instantaneous wind measurement
		ts := int64(tm.Ob[0])
		if goofy_units {
			gms = append(gms, graphiteMsg("wind_speed_mph", tm.Ob[1]*2.23694, ts))
		} else {
			gms = append(gms, graphiteMsg("wind_speed_mps", tm.Ob[1], ts))
		}
		gms = append(gms, graphiteMsg("wind_dir", tm.Ob[2], ts))
	case "obs_st":
		// tempest observation, an array of arrays per-device maybe?
		for _, ob := range tm.Obs {
			ts := int64(ob[0])
			if goofy_units {
				gms = append(gms, graphiteMsg("wind_lull_mph", ob[1]*2.23694, ts))
				gms = append(gms, graphiteMsg("wind_avg_mph", ob[2]*2.23694, ts))
				gms = append(gms, graphiteMsg("wind_gust_mph", ob[3]*2.23694, ts))
				gms = append(gms, graphiteMsg("temp_f", ob[7]*9/5+32, ts))
				gms = append(gms, graphiteMsg("rain_in", ob[12]/25.4, ts))
				gms = append(gms, graphiteMsg("light_dst_mi", ob[14]/1.60934, ts))
			} else {
				gms = append(gms, graphiteMsg("wind_lull_mps", ob[1], ts))
				gms = append(gms, graphiteMsg("wind_avg_mps", ob[2], ts))
				gms = append(gms, graphiteMsg("wind_gust_mps", ob[3], ts))
				gms = append(gms, graphiteMsg("temp_c", ob[7], ts))
				gms = append(gms, graphiteMsg("rain_mm", ob[12], ts))
				gms = append(gms, graphiteMsg("light_dst_km", ob[14], ts))
			}
			gms = append(gms, graphiteMsg("wind_dir", ob[4], ts))
			gms = append(gms, graphiteMsg("wind_int", ob[5], ts))
			gms = append(gms, graphiteMsg("pres_hpa", ob[6], ts))
			gms = append(gms, graphiteMsg("humd", ob[8], ts))
			gms = append(gms, graphiteMsg("lumn", ob[9], ts))
			gms = append(gms, graphiteMsg("uv", ob[10], ts))
			gms = append(gms, graphiteMsg("solar_rad", ob[11], ts))
			gms = append(gms, graphiteMsg("prcp_type", ob[13], ts))
			gms = append(gms, graphiteMsg("light_cnt", ob[15], ts))
			gms = append(gms, graphiteMsg("batt_volt", ob[16], ts))
			gms = append(gms, graphiteMsg("reprt_int", ob[17], ts))
		}
	case "hub_status":
		gms = append(gms,
			graphiteMsg("hub_uptime", (float64)(tm.Uptime), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("hub_rssi", (float64)(tm.Rssi), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("radio_reboot", (float64)(tm.Radio_stats[1]), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("radio_err", (float64)(tm.Radio_stats[2]), tm.Timestamp))
	case "device_status":
		gms = append(gms,
			graphiteMsg("dev_uptime", (float64)(tm.Uptime), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("dev_voltage", (float64)(tm.Voltage), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("dev_rssi", (float64)(tm.Rssi), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("dev_hrssi", (float64)(tm.Hub_rssi), tm.Timestamp))
		gms = append(gms,
			graphiteMsg("dev_status", (float64)(tm.Sensor_status), tm.Timestamp))
	}

	return gms
}

func init() {
	flag.StringVar(&listen_addr, "l", ":50222", "UDP socket to listen to")
	flag.StringVar(&graphite_addr, "g", "graphite:2003",
		"destination for graphite line-protocol UDP packets")
	flag.BoolVar(&goofy_units, "u", false, "Use goofy (imperial) units")
	flag.BoolVar(&verbose, "v", false, "Also print all packets to stdout")
	flag.Parse()
}

func main() {

	laddr, err := net.ResolveUDPAddr("udp", listen_addr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	listen_conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	send_conn, err := net.Dial("udp", graphite_addr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("sending graphite output to %s\n", graphite_addr)

	var buf [2000]byte
	var tm TempestMsg

	/* loop forever, processing each tempest udp packet we hear into
	   graphite udp packets */
	for {
		n, _, err := listen_conn.ReadFromUDP(buf[:])
		if err != nil {
			fmt.Printf("error on udp read: %s\n", err)
			continue
		}
		if err = json.Unmarshal(buf[:n], &tm); err != nil {
			fmt.Printf("error on unmarshal: %s\n", err)
			fmt.Println("> ", string(buf[:n]))
			continue
		}
		for _, msg := range decodeMsg(&tm) {
			if _, err = send_conn.Write(([]byte)(msg)); err != nil {
				fmt.Printf("error on socket write: %s\n", err)
			} else {
				if (verbose) {
					fmt.Printf("%s <- %s\n", graphite_addr, msg)
				}
			}
		}
	}

}
