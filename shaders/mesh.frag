in vec3 vWorld;
in vec3 vN;
in vec3 vColor;
in vec2 vUV;
in float vMat;
in float vTint;
in float vSway;

#ifndef SHADOW
uniform sampler2DShadow uShadow;
uniform sampler2D uRipple;
uniform sampler2D uReflect;
out vec4 oColor;
#endif

float leafKeep(vec3 wp, float tint, vec3 col) {
  float dist = length(wp - uCam.xyz);
  float evergreen = step(col.r, .055);
  float k = .27 + .5 * uSeason.z * (1. - evergreen) + .12 * uSeason.y * (1. - evergreen);
  k *= 1. - .85 * sstep(50., 190., dist);
  float n = vnoise3(wp * 2.8 + tint * 17.);
  float n2 = vnoise3(wp * 7.5 + 3.1);
  return mix(n, n2, .35) - k;
}

#ifdef SHADOW
void main() {
  float m = floor(vMat + .01);
  if (m == 1. && leafKeep(vWorld, vTint, vColor) < 0.) discard;
}
#else

float shadowAt(vec3 wp, vec3 n, float ndl) {
  if (uSunCol.w < .01) return 1.;
  vec4 lp = uLightVP * vec4(wp + n * .08, 1.);
  vec3 sc = lp.xyz / lp.w * .5 + .5;
  if (sc.x < 0. || sc.x > 1. || sc.y < 0. || sc.y > 1. || sc.z > 1.) return 1.;
  float bias = .0006 + .0012 * (1. - ndl);
  vec2 ts = 1. / vec2(textureSize(uShadow, 0));
  float sh = 0.;
  sh += texture(uShadow, vec3(sc.xy + ts * vec2(-1.2, -.4), sc.z - bias));
  sh += texture(uShadow, vec3(sc.xy + ts * vec2(1.1, -.9), sc.z - bias));
  sh += texture(uShadow, vec3(sc.xy + ts * vec2(-.7, 1.2), sc.z - bias));
  sh += texture(uShadow, vec3(sc.xy + ts * vec2(.9, .8), sc.z - bias));
  sh += texture(uShadow, vec3(sc.xy, sc.z - bias));
  sh /= 5.;
  vec2 e = abs(sc.xy - .5) * 2.;
  return mix(1., sh, 1. - sstep(.78, 1., max(e.x, e.y)));
}

vec3 pointLights(vec3 N, vec3 wp, vec3 V, float spec, float wrap) {
  vec3 acc = vec3(0.);
  for (int i = 0; i < 8; i++) {
    vec4 L = uLights[i];
    if (L.w <= 0.) continue;
    vec3 d = L.xyz - wp;
    float dist = length(d);
    if (dist > L.w) continue;
    vec3 l = d / dist;
    float att = 1. - dist / L.w;
    att *= att;
    float fl = 1. + uLightC[i].w * sin(uCam.w * 9. + float(i) * 3.7) * (.5 + .5 * sin(uCam.w * 2.3 + float(i)));
    float ndl = clamp((dot(N, l) + wrap) / (1. + wrap), 0., 1.);
    vec3 col = uLightC[i].rgb * fl;
    acc += col * att * ndl * 1.4;
    if (spec > 0.) acc += col * att * pow(max(dot(N, normalize(l + V)), 0.), 48.) * spec;
  }
  return acc;
}

vec3 seasonalGreen(vec3 c, float tint, vec3 wp) {
  float n = vnoise(wp.xz * .15 + tint * 9.);
  c = mix(c, c * vec3(1.18, 1.3, .75) + vec3(.02, .05, 0.), uSeason.w * .8);
  c = mix(c, autumnPalette(tint * 1.7 + n * .6) * (.7 + .5 * luma(c) * 2.), uSeason.y * .85);
  c = mix(c, vec3(luma(c)) * vec3(.9, .95, .9) + .06, uSeason.z * .35);
  return c;
}

vec3 screenContent(float variant, vec2 uv, float t, float mode) {
  vec3 tint = mode < .5 ? vec3(.25, 1., .55) : mode < 1.5 ? vec3(1., .7, .2) : mode < 2.5 ? vec3(.3, .8, 1.) : vec3(1., .25, .2);
  vec3 col = tint * .05;
  float scan = .85 + .15 * sin(uv.y * 240.);
  if (variant < .5) { // scrolling terminal log
    float row = floor(uv.y * 18. + t * 1.4);
    float cx = uv.x * 46.;
    float len = hash21(vec2(row, 3.)) * 30. + 8.;
    float word = step(.3, hash21(vec2(floor(cx / 4.), row)));
    float on = step(cx, len) * word * step(.2, fract(uv.y * 18. + t * 1.4)) * step(fract(uv.y * 18. + t * 1.4), .8);
    on *= step(.35, fract(cx));
    col += tint * on * (.5 + .5 * hash21(vec2(row, floor(cx))));
    col += tint * step(.96, hash21(vec2(row, 91.))) * .4;
  } else if (variant < 1.5 || (variant > 3.5 && variant < 4.5)) { // network graph
    float big = variant > 3.5 ? 1. : 0.;
    float best = 1e3, packet = 0.;
    for (int i = 0; i < 9; i++) {
      float fi = float(i);
      vec2 a = vec2(hash11(fi * 3.1), hash11(fi * 7.7)) * .8 + .1;
      a += .03 * vec2(sin(t * .7 + fi), cos(t * .5 + fi * 1.3));
      float fj = mod(fi + 3., 9.);
      vec2 b = vec2(hash11(fj * 3.1), hash11(fj * 7.7)) * .8 + .1;
      b += .03 * vec2(sin(t * .7 + fj), cos(t * .5 + fj * 1.3));
      vec2 pa = uv - a, ba = b - a;
      float h = clamp(dot(pa, ba) / dot(ba, ba), 0., 1.);
      best = min(best, length(pa - ba * h));
      col += tint * sstep(.02, .0, length(uv - a)) * 1.2;
      float pt = fract(t * .4 + fi * .17);
      packet += sstep(.015, 0., length(uv - mix(a, b, pt))) * 1.5;
    }
    col += tint * sstep(.006, .0, best) * .6 + vec3(1.) * packet * .8;
    if (big > .5) col += tint * .08 * step(.97, fract(uv.x * 20.)) + tint * .08 * step(.97, fract(uv.y * 12.));
  } else if (variant < 2.5 || variant > 4.5) { // radar
    vec2 p = (uv - .5) * vec2(1.6, 1.);
    float r = length(p) * 2.;
    float a = atan(p.y, p.x);
    float sweep = mod(t * 1.6, 6.2832) - 3.1416;
    float d = mod(sweep - a + 3.1416, 6.2832);
    float trail = exp(-d * 2.2) * step(r, 1.);
    col += tint * (trail * .8 + sstep(.015, 0., abs(r - .33)) * .5 + sstep(.015, 0., abs(r - .66)) * .5 + sstep(.015, 0., abs(r - 1.)) * .7);
    col += tint * (sstep(.01, 0., abs(p.x)) + sstep(.01, 0., abs(p.y))) * .25 * step(r, 1.);
    for (int i = 0; i < 6; i++) {
      float fi = float(i);
      float ba = hash11(fi * 5.3) * 6.2832 - 3.1416;
      float br = hash11(fi * 2.9) * .85 + .1;
      float dd = mod(sweep - ba + 3.1416, 6.2832);
      col += vec3(1., .9, .7) * sstep(.04, 0., length(p * 2. - vec2(cos(ba), sin(ba)) * br)) * exp(-dd * .8);
    }
  } else { // bar chart + waveform
    float bars = floor(uv.x * 14.);
    float bh = .2 + .7 * hash21(vec2(bars, floor(t * 1.5)));
    float bh2 = mix(.2 + .7 * hash21(vec2(bars, floor(t * 1.5) - 1.)), bh, fract(t * 1.5));
    col += tint * step(uv.y, bh2 * .55) * step(.12, fract(uv.x * 14.)) * step(fract(uv.x * 14.), .88) * .7;
    float w = .78 + .12 * sin(uv.x * 18. + t * 3.) + .05 * sin(uv.x * 47. - t * 5.);
    col += tint * sstep(.012, 0., abs(uv.y - w)) * 1.2;
  }
  return col * scan * (.9 + .1 * hash21(uv * 300. + floor(t * 30.)));
}

vec3 waterShade(vec3 wp, float depth, float river, float variant, out float alpha) {
  vec3 V = normalize(uCam.xyz - wp);
  vec2 p = wp.xz;
  float t = uCam.w;
  float windK = .35 + length(uWind.xy) * .8;
  vec2 flow = vec2(.35, .12) * (.4 + river * 1.4);
  vec2 g = vec2(0.);
  g += vec2(.6, .8) * cos(dot(p, vec2(.6, .8)) * 1.7 + t * 1.1) * .05;
  g += vec2(-.7, .7) * cos(dot(p, vec2(-.7, .7)) * 2.9 - t * 1.7) * .035;
  g += (vec2(vnoise(p * 3. + t * flow * 2.), vnoise(p * 3. + vec2(17., 3.) - t * flow * 1.7)) - .5) * .22 * windK;
  g += (vec2(vnoise(p * 9. - t * vec2(.8, .3)), vnoise(p * 9. + vec2(5., 11.) + t * vec2(.5, .9))) - .5) * .1 * windK;
  // rain / wading ripples from the simulated height field
  vec2 ruv = (wp.xz - uWater.zw) / uCamF.w;
  if (ruv.x > .02 && ruv.x < .98 && ruv.y > .02 && ruv.y < .98) {
    float e = 1. / 96.;
    float hx = texture(uRipple, ruv + vec2(e, 0.)).r - texture(uRipple, ruv - vec2(e, 0.)).r;
    float hz = texture(uRipple, ruv + vec2(0., e)).r - texture(uRipple, ruv - vec2(0., e)).r;
    g += vec2(hx, hz) * 7.;
  }
  vec3 N = normalize(vec3(-g.x, 1., -g.y));
  float ndv = max(dot(N, V), 0.);
  float fres = .02 + .98 * pow(1. - ndv, 5.);
  vec3 R = reflect(-V, N);
  R.y = abs(R.y);
  vec3 refl;
  if (uMisc.y > .5) {
    vec2 suv = gl_FragCoord.xy * uRes.zw + N.xz * .035;
    refl = texture(uReflect, vec2(suv.x, suv.y)).rgb;
  } else {
    refl = skyColor(R);
  }
  vec3 L = normalize(uSunDir.xyz);
  float light = clamp(.25 + uSunDir.w * .8 + luma(uAmb.rgb) * .6, 0., 1.6);
  vec3 deep = vec3(.02, .17, .17), shallow = vec3(.12, .45, .38);
  vec3 body = mix(shallow, deep, sstep(.1, 3.5, depth)) * light;
  body = mix(body, body * vec3(.8, .95, .7) + vec3(.02, .05, 0.), river * .3);
  vec3 col = mix(body, refl, clamp(fres * .95, 0., 1.));
  float spec = pow(max(dot(R, L), 0.), 700.) * 8. + pow(max(dot(R, L), 0.), 40.) * .15;
  col += uSunCol.rgb * spec * step(0.02, uSunDir.w);
  // shoreline foam and white water in fast rivers
  float foamN = vnoise(p * 4.5 + t * vec2(.3, .15)) * .6 + vnoise(p * 11. - t * .4) * .4;
  float shore = 1. - sstep(0., .5, depth);
  float foam = sstep(.5, .8, foamN + shore * .7 - .45) * shore * (.6 + river * .4);
  col = mix(col, vec3(.92, .97, 1.) * light, clamp(foam, 0., 1.));
  if (variant > .5) col += vec3(.1, .5, .55) * (.15 + .1 * sin(t * 1.3 + p.x * .5)) * .6;
  alpha = clamp(sstep(-.03, .45, depth) * (.55 + .4 * fres) + foam * .6, 0., .96);
  return col;
}

void main() {
  if (vWorld.y < uMisc.z) discard;
  float m = floor(vMat + .01);
  float variant = floor(fract(vMat + .01) * 10.);
  vec3 N = normalize(vN);
  if (!gl_FrontFacing) N = -N;
  vec3 wp = vWorld;
  vec3 V = normalize(uCam.xyz - wp);
  vec3 L = normalize(uSunDir.xyz);
  float dist = length(wp - uCam.xyz);
  vec3 base = vColor;
  float alpha = 1.;
  float emis = 0.;
  float spec = 0.;
  float wrap = 0.;
  float trans = 0.;
  float interiorK = 0.;  // 1 = no sun, dim ambient
  float caveK = 0.;
  float wet = uFogCol.w;

  if (m == 1.) { // foliage
    if (leafKeep(wp, vTint, vColor) < 0.) discard;
    base = seasonalGreen(base * (.88 + .24 * vnoise3(wp * 1.3)), vTint, wp);
    wrap = .35; trans = 1.;
    float snowTop = uSeason.z * sstep(.45, .85, N.y);
    base = mix(base, vec3(.9, .94, .98), snowTop * .85);
  } else if (m == 15.) { // grass
    float g = vUV.y;
    base = mix(base * .7, base * 1.15, g);
    base = seasonalGreen(base * (.85 + .3 * vTint), vTint, wp);
    base = mix(base, vec3(.88, .92, .96), uSeason.z * .8 * g);
    wrap = .5; trans = .8;
  } else if (m == 14.) { // flower petals
    base = flowerPalette(vTint) * (.9 + .15 * vnoise(wp.xz * 20.));
    base = mix(base, base * .6, uSeason.z * .7);
    wrap = .5; trans = .6;
  } else if (m == 2.) { // emissive, with optional pulse
    float pulse = 1.;
    if (variant == 1.) pulse = .65 + .35 * sin(uCam.w * 1.4 + wp.x * .3 + wp.z * .2);
    else if (variant == 2.) pulse = step(.45, hash21(vec2(floor(uCam.w * 3. + wp.x * 7. + wp.y * 13.), floor(wp.z * 9.))));
    else if (variant == 3.) pulse = .75 + .25 * vnoise(vec2(uCam.w * 6., wp.x * 3.));
    oColor = vec4(applyFog(base * 1.6 * pulse + vec3(.02), wp), 1.);
    oColor.rgb = oColor.rgb / (1. + oColor.rgb * .3);
    return;
  } else if (m == 3.) { // procedural screens
    vec3 c = screenContent(variant, vUV, uCam.w, uZone.z);
    oColor = vec4(applyFog(c * 1.6, wp), 1.);
    return;
  } else if (m == 4.) { // rain-streaked glass
    vec2 gp = vec2(wp.x + wp.z, wp.y) * 7.;
    float rainAmt = max(uWind.w, .55);
    float cell = hash21(floor(gp));
    vec2 f = fract(gp) - .5;
    float drop = sstep(.22, .08, length(f - vec2(0., .2 * sin(cell * 6.28 + uCam.w * .6 * cell)))) * step(cell, rainAmt);
    float streak = sstep(.06, .0, abs(f.x - (cell - .5) * .5)) * step(.7, cell) * fract(gp.y * .3 + uCam.w * .15 * cell + cell);
    float fres = pow(1. - max(dot(N, V), 0.), 3.);
    vec3 c = skyColor(reflect(-V, N)) * (.25 + fres) + vec3(.55, .75, .8) * (drop * .4 + streak * .25);
    alpha = clamp(.14 + fres * .4 + drop * .35 + streak * .2, 0., .8);
    oColor = vec4(applyFog(c, wp), alpha);
    return;
  } else if (m == 5.) { // waterfall sheet / foam
    float t = uCam.w;
    if (variant == 1.) {
      vec2 q = vUV - .5;
      float r = length(q);
      float ring = .5 + .5 * sin(r * 40. - t * 5.);
      float n = vnoise(vUV * 18. + t * vec2(.5, -.3));
      float a = sstep(.5, .1, r) * (.35 + .5 * n) * (.6 + .4 * ring);
      oColor = vec4(applyFog(vec3(.95, 1., 1.) * (.6 + .6 * uSunDir.w), wp), clamp(a, 0., .85));
      return;
    }
    float s1 = vnoise(vec2(vUV.x * 14., vUV.y * 5. - t * 3.2));
    float s2 = vnoise(vec2(vUV.x * 31. + 5., vUV.y * 9. - t * 5.6));
    float edge = sstep(0., .12, vUV.x) * sstep(1., .88, vUV.x);
    float streak = sstep(.35, .8, s1 * .6 + s2 * .5);
    vec3 c = mix(vec3(.45, .85, .75), vec3(1.), streak * .85 + vUV.y * .35);
    c *= .6 + .6 * clamp(uSunDir.w + luma(uAmb.rgb), 0., 1.2);
    alpha = clamp((.5 + .35 * streak) * edge + vUV.y * .2, 0., .9);
    oColor = vec4(applyFog(c, wp), alpha);
    return;
  } else if (m == 6.) { // crystals
    float fres = pow(1. - max(dot(N, V), 0.), 2.5);
    float pulse = .7 + .3 * sin(uCam.w * 1.7 + wp.x * .8 + wp.z * .6);
    base = vColor * (.35 + 1.3 * fres * pulse + .5 * pulse) ;
    oColor = vec4(applyFog(base, wp), 1.);
    return;
  } else if (m == 7.) { // bark
    float streak = vnoise3(vec3(wp.x * 5., wp.y * .7, wp.z * 5.));
    base *= .7 + .55 * streak;
    float moss = sstep(.1, .7, N.y + vnoise(wp.xz * .4 + wp.y * .1) * .5 - .15) * (.35 + .5 * wet);
    base = mix(base, vec3(.12, .28, .09), moss * .55);
    base = mix(base, vec3(.88, .92, .96), uSeason.z * sstep(.55, .9, N.y) * .8);
  } else if (m == 8.) { // stone with moss
    float grain = vnoise3(wp * 2.2) * .6 + vnoise3(wp * 7.) * .4;
    base *= .7 + .5 * grain;
    float moss = sstep(.2, .75, N.y + (vnoise(wp.xz * .5 + wp.y * .3) - .5) * .7) * (.55 + .4 * wet);
    base = mix(base, vec3(.11, .3, .09) * (.8 + .4 * grain), clamp(moss * .75, 0., .85));
    base = mix(base, vec3(.9, .93, .97), uSeason.z * sstep(.5, .9, N.y) * .85);
    spec = wet * .5;
  } else if (m == 9.) { // terrain
    float rock = vColor.x, litter = vColor.y, shore = vColor.z;
    float path = vUV.x, zs = vUV.y, zid = vSway;
    float df = 1. - sstep(50., 200., dist);
    float n1 = fbm2(wp.xz * .35);
    float n2 = mix(.5, vnoise(wp.xz * 7.), df);
    float n3 = vnoise(wp.xz * .04);
    vec3 grass = mix(vec3(.09, .29, .07), vec3(.23, .47, .1), clamp(n1 * 1.1 + n2 * .35 - .15, 0., 1.));
    grass = mix(grass, vec3(.38, .42, .15), sstep(.55, .85, n3) * .5);
    grass = seasonalGreen(grass, n3, wp);
    vec3 rockC = mix(vec3(.33, .32, .3), vec3(.56, .53, .48), vnoise(wp.xz * .7 + wp.y * .25));
    rockC *= .85 + .3 * sin(wp.y * 2.6 + vnoise(wp.xz * .3) * 4.);
    vec3 litterC = mix(vec3(.21, .14, .07), vec3(.12, .2, .07), n2);
    vec3 sandC = mix(vec3(.4, .33, .2), vec3(.3, .27, .17), n2);
    vec3 pathC = vec3(.4, .3, .18) * (.82 + .3 * n2);
    vec3 c = mix(grass, litterC, litter * (.5 + .5 * n1));
    c = mix(c, sandC, shore);
    c = mix(c, rockC, clamp(rock, 0., 1.));
    c = mix(c, pathC, path * (1. - rock * .7));
    if (zs > .001) {
      vec3 zc = c;
      if (abs(zid - 6.) < .5) { // ruins: cracked paving
        vec2 cell = floor(wp.xz * .55);
        float tile = hash21(cell);
        zc = mix(vec3(.4, .4, .37), vec3(.5, .49, .44), tile) * (.75 + .3 * n2);
        zc = mix(zc, grass * .8, sstep(.45, .75, n1 + (tile - .5) * .3));
      } else if (abs(zid - 9.) < .5) { // world tree grove: glowing veins in the loam
        vec2 vp = wp.xz * .22;
        float vein = sstep(.06, .0, abs(vnoise(vp) - .5) * .5);
        zc = mix(grass * .9, litterC, .5);
        emis = vein * (.35 + .25 * sin(uCam.w * 1.2 + wp.x * .1)) * 1.;
        zc += vec3(.1, .55, .4) * vein * .6;
      } else if (abs(zid - 8.) < .5) { // station: damp concrete pad near the building
        zc = mix(c, vec3(.28, .3, .29) * (.8 + .3 * n2), sstep(.55, .9, zs) * .7);
      } else if (abs(zid - 7.) < .5) {
        zc = mix(c, rockC * 1.05, sstep(.2, .7, zs));
      } else if (abs(zid - 1.) < .5) {
        zc = mix(c, grass * 1.12, .35);
      }
      c = mix(c, zc, clamp(zs * 1.4, 0., 1.));
    }
    float snow = max(uSeason.z, sstep(135., 190., wp.y) * .9) * sstep(.55, .9, N.y + (n1 - .5) * .35);
    c = mix(c, vec3(.9, .93, .97), clamp(snow, 0., 1.));
    base = c * (1. - .22 * wet);
    spec = wet * .45 * (1. - rock * .4);
    if (uWater.x < .5 && wp.y < uWater.y) base *= vec3(.7, .9, .95);
  } else if (m == 10.) { // water
    float a;
    vec3 c = waterShade(wp, vUV.x, vUV.y, variant, a);
    oColor = vec4(applyFog(c, wp), a);
    oColor.rgb = oColor.rgb / (1. + oColor.rgb * .25);
    return;
  } else if (m == 11.) { // metal
    float fres = pow(1. - max(dot(N, V), 0.), 3.);
    vec3 R = reflect(-V, N);
    base = mix(base, skyColor(R) * .8, .25 + .35 * fres);
    spec = 1.;
    wet = max(wet, .3);
  } else if (m == 12.) { // glowing fungi
    vec3 c = vColor;
    if (vTint >= 0.) c = mix(vColor, hsv2rgb(vec3(.42 + vTint * .35, .65, 1.)), .6);
    float pulse = .75 + .25 * sin(uCam.w * 1.5 + wp.x * 2. + wp.z * 1.7);
    oColor = vec4(applyFog(c * (.9 + 1.1 * pulse), wp), 1.);
    oColor.rgb = oColor.rgb / (1. + oColor.rgb * .3);
    return;
  } else if (m == 13.) { // wood
    float grain = vnoise(vec2(wp.x * 2. + wp.z * 2., wp.y * 14.));
    base *= .75 + .4 * grain;
    spec = wet * .3;
  } else if (m == 16.) { // cave rock
    interiorK = 1.; caveK = 1.;
    float n = vnoise3(wp * .8) * .6 + vnoise3(wp * 3.) * .4;
    base *= .6 + .8 * n;
    spec = .9;
  } else if (m == 17.) { // indoor surfaces
    interiorK = 1.;
    base *= .85 + .3 * vnoise3(wp * 3.);
    spec = .2;
  } else { // standard
    base *= .94 + .1 * vnoise3(wp * 4.);
    spec = wet * .2;
  }

  float ndl = max(dot(N, L), 0.);
  float sh = (interiorK > .5) ? 0. : shadowAt(wp, N, ndl);
  float sunK = uSunDir.w;
  float wl = clamp((dot(N, L) + wrap) / (1. + wrap), 0., 1.);
  vec3 sunLight = uSunCol.rgb * wl * sh * (interiorK > .5 ? 0. : 1.);
  vec3 amb = mix(uAmb.rgb * vec3(.55, .5, .38), uAmb.rgb, N.y * .5 + .5);
  if (interiorK > .5) amb = caveK > .5 ? vec3(.018, .026, .04) : vec3(.07, .062, .055);
  vec3 lit = sunLight + amb;
  lit += pointLights(N, wp, V, spec, wrap + (interiorK > .5 ? .25 : 0.));
  vec3 col = base * lit;
  if (trans > 0.) col += base * uSunCol.rgb * pow(max(dot(-L, V), 0.), 4.) * .45 * trans * (1. - ndl) * sh;
  if (spec > 0. && interiorK < .5) {
    vec3 H = normalize(L + V);
    col += uSunCol.rgb * pow(max(dot(N, H), 0.), 60.) * spec * .35 * sh;
  }
  col += emis * vec3(.2, 1., .75);
  // lightning flash and general storm darkening happen through uniforms; flash adds ambient
  col += uParams.w * vec3(.5, .55, .8) * (.5 + .5 * N.y) * (interiorK > .5 ? .1 : 1.) * base;
  col = applyFog(col, wp);
  col = col / (1. + col * .22);
  oColor = vec4(col, alpha);
}
#endif
