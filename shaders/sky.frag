in vec2 vNdc;
out vec4 oColor;

float cloudField(vec2 p, float t) {
  float a = .5, s = 0.;
  vec2 q = p;
  for (int i = 0; i < 5; i++) { s += a * vnoise(q + t * (.012 + .01 * float(i))); q = q * 2.1 + 9.7; a *= .5; }
  return s;
}

void main() {
  vec3 dir = normalize(uCamF.xyz + uCamR.xyz * vNdc.x * uCamR.w + uCamU.xyz * vNdc.y * uCamU.w);
  if (uWater.x > .5) {
    oColor = vec4(vec3(.02, .16, .2) * (.25 + .75 * clamp(uSunDir.w, 0., 1.)) + vec3(.05, .2, .22) * clamp(dir.y, 0., 1.) * uSunDir.w, 1.);
    return;
  }
  float t = uCam.w;
  float h = dir.y;
  vec3 col = mix(uSkyHor.rgb, uSkyTop.rgb, pow(clamp(h, 0., 1.), .5));
  col = mix(col, uFogCol.rgb, exp(-max(h, 0.) * 7.) * .55);
  vec3 sunD = normalize(uSunDir.xyz);
  float sun = uSunDir.w;
  float sd = max(dot(dir, sunD), 0.);
  float night = uMoonDir.w;
  float cover = uParams.z;

  // sun disc and glow
  if (sun > .02) {
    float disc = sstep(.99955, .9998, sd);
    col += uSunCol.rgb * (pow(sd, 180.) * .55 + pow(sd, 10.) * .2 + disc * 14.) * (1. - cover * .75);
  }
  // stars, milky way, shooting stars
  float sv = uParams.y * (1. - cover * .85) * sstep(-.02, .12, h);
  if (sv > .01) {
    vec3 sp = dir * 140.;
    vec3 id = floor(sp);
    vec3 f = fract(sp) - .5;
    float r = hash31(id);
    float star = step(.982, r) * sstep(.22, .0, length(f + (hash13(r * 91.) - .5) * .5));
    float tw = .65 + .35 * sin(t * (2. + r * 4.) + r * 60.);
    vec3 sc = mix(vec3(.8, .9, 1.), vec3(1., .85, .7), fract(r * 37.));
    col += sc * star * tw * 1.8 * sv * (1. + uZone.w * 1.5);
    vec3 sp2 = dir * 60.;
    vec3 id2 = floor(sp2);
    float r2 = hash31(id2 + 7.3);
    col += vec3(.9, .95, 1.) * step(.992, r2) * sstep(.2, .0, length(fract(sp2) - .5)) * 1.4 * sv;
    float band = exp(-pow(dot(dir, normalize(vec3(.3, .8, .5))), 2.) * 14.);
    float dust = vnoise3(dir * 9.) * .7 + vnoise3(dir * 22.) * .3;
    col += vec3(.16, .18, .28) * band * (.3 + dust) * sv * (.5 + uZone.w);
    float cyc = floor(t / 7.);
    float u = fract(t / 7.) / .14;
    if (u < 1.) {
      float hh = hash11(cyc);
      vec3 a = normalize(vec3(hh - .5, .45 + .4 * hash11(cyc + 3.), hash11(cyc + 9.) - .5));
      vec3 b = normalize(a + vec3(.5 - hash11(cyc + 5.), -.22, .3));
      vec3 head = normalize(mix(a, b, u));
      vec3 tail = normalize(mix(a, b, max(u - .35, 0.)));
      vec3 e = head - tail;
      float k = clamp(dot(dir - tail, e) / max(dot(e, e), 1e-6), 0., 1.);
      float d = length(dir - (tail + e * k));
      col += vec3(1., .95, .85) * sstep(.0035, 0., d) * k * (1. - u) * 2.2 * sv;
    }
  }
  // moon with phase
  float md = dot(dir, uMoonDir.xyz);
  if (md > .995 && uMoonDir.w > .02) {
    float R = .0305;
    vec3 q = (dir - uMoonDir.xyz * md) / R;
    float r2 = dot(q, q);
    if (r2 < 1.) {
      vec3 n = q + uMoonDir.xyz * sqrt(1. - r2);
      vec3 side = normalize(cross(uMoonDir.xyz, vec3(0., 1., 0.)));
      float ang = uParams.x * 6.2832;
      vec3 ld = normalize(-uMoonDir.xyz * cos(ang) + side * sin(ang));
      float lit = sstep(-.04, .08, dot(n, ld));
      float cr = vnoise3(n * 5.) * .6 + vnoise3(n * 13.) * .4;
      vec3 mc = vec3(.93, .92, .86) * (.62 + .38 * sstep(.3, .7, cr));
      col = mix(col, mc * lit + vec3(.03, .04, .07), sstep(1., .96, r2));
    }
  }
  col += vec3(.55, .65, 1.) * pow(max(md, 0.), 60.) * .12 * uMoonDir.w * (1. - cover * .6);
  // clouds
  if (h > -.02) {
    vec2 cp = dir.xz / (max(h, 0.) + .22) * 1.15 + uWind.xy * t * .006;
    float d = cloudField(cp, t);
    float thr = 1. - cover * .78 - .1;
    float c = sstep(thr, thr + .22, d);
    float d2 = cloudField(cp + normalize(sunD.xz + 1e-4) * .09, t);
    float shade = clamp(.55 + (d - d2) * 4., 0., 1.2);
    float storm = sstep(.65, 1., cover);
    vec3 lit = uSunCol.rgb * (.55 + .5 * shade) * (1. - .55 * storm) + uAmb.rgb * 1.1 + uSkyHor.rgb * .25;
    vec3 cc = mix(lit, vec3(.2, .22, .26) * (.5 + uSunDir.w * .5), storm * .75);
    cc += uSunCol.rgb * pow(sd, 6.) * .35 * (1. - storm);
    cc += vec3(.55, .6, .85) * uParams.w * 1.2;
    cc = mix(cc, vec3(luma(cc)) , uSeason.z * .2);
    col = mix(col, cc, c * sstep(-.02, .22, h) * (.45 + .55 * min(cover * 1.3, 1.)));
  }
  col += vec3(.45, .5, .8) * uParams.w * (.4 + cover);
  col = col / (1. + col * .12);
  oColor = vec4(col, 1.);
}
