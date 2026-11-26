layout(location = 0) in vec2 aID; // x particle index, y line end flag
uniform int uKind;
out vec4 vCol;
out float vAng;

vec3 wrapBox(vec3 p, vec3 box) {
  return mod(p - uCam.xyz + box * .5, box) - box * .5 + uCam.xyz;
}

void main() {
  float id = aID.x;
  vec3 r = hash13(id);
  vec3 r2 = hash13(id + 101.7);
  float t = uCam.w;
  float ground = uFogP.z;
  vec3 p;
  float size = 2.;
  vec4 col = vec4(1.);
  vAng = 0.;
  float open = uAmb.w;
  float under = step(.5, uWater.x);
  if (uKind == 0) { // rain streaks
    vec3 box = vec3(44., 28., 44.);
    vec3 vel = vec3(uWind.x * 7., -18. - r2.x * 5., uWind.y * 7.);
    vec3 base = vec3(r.x * box.x, r.y * box.y, r.z * box.z);
    p = wrapBox(base + vel * t, box);
    p.y = uCam.y - 8. + mod(base.y + vel.y * t, box.y);
    p -= normalize(vel) * .6 * aID.y;
    col = vec4(.72, .82, .95, .38 * open * (1. - under) * (aID.y < .5 ? 1. : .2));
  } else if (uKind == 1) { // snow
    vec3 box = vec3(40., 26., 40.);
    vec3 base = r * box;
    vec3 vel = vec3(uWind.x * 2.5, -1.4 - r2.x, uWind.y * 2.5);
    p = wrapBox(base + vel * t, box);
    p.y = uCam.y - 8. + mod(base.y + vel.y * t, box.y);
    p.xz += vec2(sin(t * .7 + id), cos(t * .6 + id * 1.7)) * .6;
    size = 3. + r2.y * 3.;
    col = vec4(.95, .97, 1., .9 * open * (1. - under));
  } else if (uKind == 2) { // pollen
    vec3 box = vec3(70., 14., 70.);
    p = wrapBox(vec3(r.x * box.x, 0., r.z * box.z) + vec3(uWind.x, 0., uWind.y) * t * 1.4 + vec3(sin(t * .2 + id), 0., cos(t * .17 + id * 1.3)) * 2., box);
    p.y = ground + .5 + r.y * 11. + sin(t * .4 + id) * .6;
    size = 2.4 + r2.x * 1.6;
    col = vec4(1., .95, .62, (.35 + .45 * sin(t * 1.3 + id) * .5 + .25) * uSunDir.w * open * (1. - under));
  } else if (uKind == 3) { // dust motes
    vec3 box = vec3(36., 12., 36.);
    p = wrapBox(vec3(r.x * box.x, 0., r.z * box.z) + vec3(uWind.x, 0., uWind.y) * t * .5, box);
    p.y = ground + 1. + r.y * 9. + sin(t * .3 + id * 2.) * .4;
    size = 1.6 + r2.x;
    col = vec4(vec3(1., .96, .85), .22 * uSunDir.w * open * (1. - under));
  } else if (uKind == 4) { // falling leaves
    vec3 box = vec3(60., 32., 60.);
    vec3 base = r * box;
    vec3 vel = vec3(uWind.x * 3. + sin(t * .5 + id) * .8, -1.15 - r2.x * .6, uWind.y * 3. + cos(t * .4 + id * 1.3) * .8);
    p = wrapBox(base + vel * t, box);
    p.y = ground - 1. + mod(base.y + vel.y * t, box.y);
    size = 6. + r2.y * 4.;
    vAng = t * (1. + r2.z * 2.) + id;
    vec3 lc = uSeason.y > .3 ? autumnPalette(r2.x) : vec3(.25, .5, .12) * (.8 + .6 * r2.y);
    col = vec4(lc, .95 * (1. - under));
  } else if (uKind == 5) { // fireflies
    vec3 box = vec3(80., 8., 80.);
    p = wrapBox(vec3(r.x * box.x, 0., r.z * box.z), box);
    p.xz += vec2(sin(t * .3 + r.x * 20.) + sin(t * .71 + r.y * 9.), cos(t * .37 + r.y * 30.) + cos(t * .63 + r.z * 11.)) * 2.4;
    p.y = ground + .5 + r.y * 4.5 + sin(t * .5 + r.z * 20.) * .5;
    float pulse = sstep(.5, 1., sin(t * (1. + r2.x * 1.4) + r.z * 40.) * .5 + .5);
    size = 3. + 7. * pulse;
    col = vec4(.88, 1., .35, pulse * open * (1. - under) * (.4 + .6 * uMoonDir.w));
  } else { // glowing spores
    vec3 box = vec3(50., 14., 50.);
    p = wrapBox(vec3(r.x * box.x, 0., r.z * box.z) + vec3(sin(t * .15 + id), 0., cos(t * .12 + id)) * 3., box);
    p.y = ground + .3 + r.y * 12. + sin(t * .35 + id) * .8;
    float pulse = .5 + .5 * sin(t * 1.1 + id * 3.);
    size = 3. + 4. * pulse;
    vec3 sc = r2.x < .4 ? vec3(.3, 1., .75) : r2.x < .75 ? vec3(.4, .7, 1.) : vec3(.75, .45, 1.);
    col = vec4(sc, .35 + .6 * pulse);
  }
  float dist = distance(p, uCam.xyz);
  col.a *= 1. - sstep(30., 60., dist) * (uKind == 0 ? 0. : 1.);
  gl_PointSize = clamp(size * (uRes.y / 760.) * clamp(9. / max(dist, 1.), .25, 1.6), 1., 28.);
  vCol = col;
  gl_Position = uVP * vec4(p, 1.);
}
