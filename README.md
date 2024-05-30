# tempestbridge

Forwards Weatherflow Tempest data to graphite/carbon.

The Weatherflow Tempest hub blasts out UDP broadcast packets to
255.255.255.255 port 50222 each time it receives data from its
sensors.  (I'm not thrilled with this, especially since its only
network interface is 2.4GHz wifi; I hope some future version lets you
specify a destination address or turn it off.)  These are
JSON-formatted and documented here:

https://apidocs.tempestwx.com/reference/tempest-udp-broadcast

This program listens to udp port 50222, parses the JSON, and
translates the observations into graphite "plaintext protocol," which
is described here:

https://graphite.readthedocs.io/en/latest/feeding-carbon.html

The result will be timeseries with the following names:

+ wx.tempest.temp_c
+ wx.tempest.pres_hpa
+ wx.tempest.wind_speed_mph
+ wx.tempest.wind_dir

And so on.

The -u option changes output to "goofy units", which means: m/s
becomes mph, degrees C becomes degrees F, km becomes miles.  The
timeseries name changes with the unit so that it is difficult to
create a corrupted timeseries with mixed units.

## Example

go run github.com/mdickers47/tempestbridge -u -g local.graphite.server:2003
