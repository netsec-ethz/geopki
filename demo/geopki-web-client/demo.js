const DEFAULT_QUERY_RADIUS = 100;
const MAX_QUERY_RADIUS = 255;
let setViewAfterLocate = false;

// function for hashing a string, copied from https://stackoverflow.com/a/3426956
function hashCode(str) {
  // java String#hashCode
  var hash = 0;
  for (var i = 0; i < str.length; i++) {
    hash = str.charCodeAt(i) + ((hash << 5) - hash);
  }
  return hash;
}
// function for turning an integer into a hex color, copied from https://stackoverflow.com/a/3426956
function intToRGB(i) {
  var c = (i & 0x00ffffff).toString(16).toUpperCase();

  return "00000".substring(0, 6 - c.length) + c;
}

// function for fetching and displaying certificates
async function fetchCertificates(longitude, latitude, altitude, radius) {
  while (!window.loadedWasm) {
    return new Promise((resolve, reject) =>
      setTimeout(() => {
        fetchCertificates(longitude, latitude, altitude, radius)
          .then(resolve)
          .catch(reject);
      }, 300)
    );
  }
  const certificates = await window
    // set manually to e.g. http://server.tyratox.ch:1234 if you want to use a different server
    .getJSONCertificates(
      location.protocol + "//" + location.host,
      longitude,
      latitude,
      altitude,
      radius
    )
    .then((jsonCertificates) => jsonCertificates.map(JSON.parse));

  if (window.geoJsonLayers) {
    for (const geoJsonLayer of geoJsonLayers) {
      map.removeLayer(geoJsonLayer);
    }
  }

  window.geoJsonLayers = [];

  if (certificates.length == 0) {
    alert(`Query returned zero responses.`);
  }

  for (const certificate of certificates) {
    // https://leafletjs.com/examples/geojson/
    for (i = 0; i < certificate.areas.length; i++) {
      const area = certificate.areas[i];
      const [minAltitude, maxAltitude] = certificate.areas_altitude[i];

      const geojsonLayer = L.geoJSON(
        {
          type: "Feature",
          properties: {
            domain: certificate.domain,
            certificate_id: certificate.certificate_id,
            not_valid_after: certificate.not_valid_after,
          },
          geometry: area,
        },
        {
          style: {
            color: "#" + intToRGB(hashCode(certificate.domain)),
          },
        }
      );

      geojsonLayer
        .bindPopup(function (layer) {
          const popup = L.DomUtil.create("div", "info-window");
          popup.innerHTML = `
      <ul class="feature-props">
      <li><strong>Domain:</strong> <code>${certificate.domain}</code></li>
      <li><strong>Min Altitude:</strong> <time>${Math.round(
        minAltitude
      )}m</time></li>
      <li><strong>Max Altitude:</strong> <time>${Math.round(
        maxAltitude
      )}m</time></li>
      <li><strong>Expiration Date:</strong> <time>${
        certificate.not_valid_after
      }</time></li>
      <li><strong>Certificate Id:</strong> <code>${
        certificate.certificate_id
      }</code></li>
      </ul>
      <br>
      <button class='bring-to-back'>Bring to Back</button>
      <button class='remove'>Remove</button>`;

          popup
            .querySelector(".bring-to-back")
            .addEventListener("click", () => {
              geojsonLayer.closePopup();
              geojsonLayer.bringToBack();
            });

          popup.querySelector(".remove").addEventListener("click", () => {
            geojsonLayer.closePopup();
            geojsonLayer.remove();
          });

          return popup;
        })
        .addTo(map);

      geoJsonLayers.push(geojsonLayer);
    }
  }

  // const bounds = L.featureGroup(geoJsonLayers).getBounds();
  // flyToBounds bugs out (https://github.com/Leaflet/Leaflet/issues/6050)
  // flyTo bugs out (https://github.com/Leaflet/Leaflet/issues/6050)
  // map.fitBounds(bounds);
  // map.setView(bounds.getCenter(), 19);
}

async function fetchCertificatesForCurrentMapLocation() {
  const center = map.getCenter();
  const radius = DEFAULT_QUERY_RADIUS;

  const certificates = await fetchCertificates(
    center.lng,
    center.lat,
    0,
    radius
  );

  if (window.positionMarker) {
    window.positionMarker.remove();
    window.positionAccuracyMarker.remove();
  }

  window.positionMarker = L.marker(center);
  window.positionMarker.addTo(map);

  window.positionAccuracyMarker = L.circle(center, radius);
  window.positionAccuracyMarker.addTo(map);
  window.positionAccuracyMarker.bringToBack();

  return certificates;
}

function onCurrentLocation() {
  setViewAfterLocate = false;
  map.locate({ watch: false, maxZoom: 19, enableHighAccuracy: true });
}

function onCurrentMapLocation() {
  map.stopLocate();
  fetchCertificatesForCurrentMapLocation().catch((e) => alert(e.message));
}

async function onLocationFound(e) {
  // https://leafletjs.com/reference.html#map-locationfound
  const longitude = e.latlng.lng;
  const latitude = e.latlng.lat;
  const altitude = e.altitude || 0;
  const radius = Math.min(e.accuracy / 2, MAX_QUERY_RADIUS);

  try {
    await fetchCertificates(longitude, latitude, altitude, radius);
  } catch (e) {
    alert(e.message);
  }

  if (window.positionMarker) {
    window.positionMarker.remove();
    window.positionAccuracyMarker.remove();
  }

  window.positionMarker = L.marker(e.latlng);
  window.positionMarker.addTo(map);

  window.positionAccuracyMarker = L.circle(e.latlng, radius);
  window.positionAccuracyMarker.addTo(map);
  window.positionAccuracyMarker.bringToBack();

  if (!setViewAfterLocate) {
    setViewAfterLocate = true;
    // fit accuracy circle
    map.fitBounds(window.positionAccuracyMarker.getBounds());
    // map.setView(e.latlng, 19);
  }
}

function onLocationError(e) {
  alert(e.message);
}
