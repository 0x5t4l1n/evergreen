// Shared declarations for every Evergreen program. The Go side prepends the
// version directive, precision qualifiers and defines.

layout(std140) uniform Frame {
  mat4 uVP;
  mat4 uLightVP;
  vec4 uCam;     // xyz camera, w time (s)
  vec4 uSunDir;  // xyz toward sun, w intensity
  vec4 uSunCol;  // rgb sun colour, w shadow strength
  vec4 uSkyTop;
  vec4 uSkyHor;
  vec4 uFogCol;  // rgb fog colour, w ground wetness
  vec4 uAmb;     // rgb ambient, w open-air factor (rain mask)
  vec4 uFogP;    // x density, y height falloff, z ground reference, w underwater density
  vec4 uWind;    // xy wind vector, z gust, w rain
  vec4 uSeason;  // x index 0..3, y autumn, z snow, w spring
  vec4 uParams;  // x moon phase, y star visibility, z cloud cover, w lightning flash
  vec4 uMoonDir; // xyz, w night
  vec4 uRes;     // scene w, h, 1/w, 1/h
  vec4 uWater;   // x underwater, y plane Y, zw ripple origin xz
  vec4 uZone;    // x zone id, y zone blend, z station mode, w stargaze
  vec4 uCamR;    // right, tan(fovx/2)
  vec4 uCamU;    // up, tan(fovy/2)
  vec4 uCamF;    // forward, ripple span
  vec4 uGrade;   // rgb grade, saturation
  vec4 uMisc;    // x grass fade distance, y planar reflection on, z clip Y, w growth time
  vec4 uLights[8];
  vec4 uLightC[8];
};

const float PI = 3.14159265;
// smoothstep with defined behaviour for reversed edges (the built-in is undefined there)
float sstep(float a, float b, float x) { float t = clamp((x - a) / (b - a), 0., 1.); return t * t * (3. - 2. * t); }

float hash11(float p) { p = fract(p * .1031); p *= p + 33.33; p *= p + p; return fract(p); }
float hash21(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * .1031); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.x + p3.y) * p3.z); }
float hash31(vec3 p3) { p3 = fract(p3 * .1031); p3 += dot(p3, p3.zyx + 31.32); return fract((p3.x + p3.y) * p3.z); }
vec3 hash13(float p) {
  vec3 p3 = fract(vec3(p) * vec3(.1031, .1030, .0973));
  p3 += dot(p3, p3.yzx + 33.33);
  return fract((p3.xxy + p3.yzz) * p3.zyx);
}
float vnoise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  f = f * f * (3. - 2. * f);
  return mix(mix(hash21(i), hash21(i + vec2(1, 0)), f.x), mix(hash21(i + vec2(0, 1)), hash21(i + vec2(1, 1)), f.x), f.y);
}
float vnoise3(vec3 p) {
  vec3 i = floor(p), f = fract(p);
  f = f * f * (3. - 2. * f);
  float a = mix(mix(hash31(i), hash31(i + vec3(1, 0, 0)), f.x), mix(hash31(i + vec3(0, 1, 0)), hash31(i + vec3(1, 1, 0)), f.x), f.y);
  float b = mix(mix(hash31(i + vec3(0, 0, 1)), hash31(i + vec3(1, 0, 1)), f.x), mix(hash31(i + vec3(0, 1, 1)), hash31(i + vec3(1, 1, 1)), f.x), f.y);
  return mix(a, b, f.z);
}
float fbm2(vec2 p) {
  float a = .5, s = 0.;
  for (int i = 0; i < 4; i++) { s += a * vnoise(p); p = p * 2.03 + 17.1; a *= .5; }
  return s;
}
vec3 hsv2rgb(vec3 c) {
  vec3 p = abs(fract(c.xxx + vec3(1., 2. / 3., 1. / 3.)) * 6. - 3.);
  return c.z * mix(vec3(1.), clamp(p - 1., 0., 1.), c.y);
}
// pastel meadow palette indexed by a 0..1 tint
vec3 flowerPalette(float t) {
  t = fract(t) * 6.;
  if (t < 1.) return vec3(1., .95, .55);
  if (t < 2.) return vec3(1., 1., 1.);
  if (t < 3.) return vec3(1., .62, .78);
  if (t < 4.) return vec3(.72, .6, 1.);
  if (t < 5.) return vec3(.5, .7, 1.);
  return vec3(1., .55, .35);
}
vec3 autumnPalette(float t) {
  t = fract(t) * 4.;
  if (t < 1.) return vec3(.78, .55, .08);
  if (t < 2.) return vec3(.7, .26, .06);
  if (t < 3.) return vec3(.55, .4, .06);
  return vec3(.8, .66, .14);
}
float luma(vec3 c) { return dot(c, vec3(.299, .587, .114)); }

// cheap sky used for reflections and fog tinting (no clouds or stars)
vec3 skyColor(vec3 d) {
  float h = clamp(d.y, -.2, 1.);
  vec3 c = mix(uSkyHor.rgb, uSkyTop.rgb, pow(max(h, 0.), .55));
  float sd = max(dot(d, normalize(uSunDir.xyz)), 0.);
  c += uSunCol.rgb * (pow(sd, 900.) * 6. + pow(sd, 12.) * .25) * step(0.02, uSunDir.w);
  float md = max(dot(d, uMoonDir.xyz), 0.);
  c += vec3(.7, .8, 1.) * pow(md, 1500.) * 3. * uMoonDir.w;
  return c;
}

// distance + low-lying fog, with sun in-scattering
vec3 applyFog(vec3 col, vec3 wp) {
  vec3 v = wp - uCam.xyz;
  float dist = length(v);
  if (uWater.x > .5) {
    float f = 1. - exp(-dist * uFogP.w);
    vec3 deep = vec3(.02, .16, .2) * (.25 + .75 * clamp(uSunDir.w, 0., 1.));
    return mix(col * vec3(.55, .85, 1.), deep, f);
  }
  float low = exp(-max(wp.y - uFogP.z, 0.) * uFogP.y);
  float dens = uFogP.x * (1. + 3.2 * low);
  float f = 1. - exp(-dist * dens);
  vec3 dir = v / max(dist, .001);
  float sun = pow(max(dot(dir, normalize(uSunDir.xyz)), 0.), 8.);
  vec3 fc = uFogCol.rgb + uSunCol.rgb * sun * .35 * f;
  return mix(col, fc, clamp(f, 0., 1.));
}
