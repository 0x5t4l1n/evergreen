layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aNormal;
layout(location = 2) in vec3 aColor;
layout(location = 3) in float aSway;
layout(location = 4) in vec2 aUV;
layout(location = 5) in float aMat;
layout(location = 6) in vec4 iA; // xyz position, w scale
layout(location = 7) in vec4 iB; // yaw, tint, birth|phase, amplitude

out vec3 vWorld;
out vec3 vN;
out vec3 vColor;
out vec2 vUV;
out float vMat;
out float vTint;
out float vSway;

void main() {
  float m = floor(aMat + .01);
  float scale = iA.w;
  vec3 local = aPos;
  float birth = iB.z;
  bool entity = birth < -500.;
  float phase = entity ? -(birth + 1000.) : 0.;
  if (!entity) scale *= sstep(0., 1., clamp((uCam.w - birth) / max(uMisc.w, .1), 0., 1.));
  if (m == 15. || m == 14.) {
    float d = distance(iA.xyz, uCam.xyz);
    float fd = m == 15. ? uMisc.x : uMisc.x * 1.35;
    scale *= 1. - sstep(fd * .7, fd, d);
  }
  float amp = iB.w;
  if (entity && amp > 0.) {
    if (aSway < 0. && aSway > -1.01) {
      local.y += sin(phase) * (-aSway) * abs(local.z) * .9 * min(amp, 1.2);
    } else if (aSway >= 2. && aSway < 3.) {
      float w = aSway - 2.;
      float s = sin(phase + (aPos.x * aPos.z > 0. ? 0. : 3.14159));
      local.x += s * w * .3 * min(amp, 1.5);
      local.y += max(0., -s) * w * .06 * min(amp, 1.5);
    } else if (aSway >= 3. && aSway < 4.) {
      float w = aSway - 3.;
      local.z += sin(phase - aPos.x * 7.) * w * .16 * min(amp, 1.);
    }
  }
  vec3 p = local * scale;
  float c = cos(iB.x), s = sin(iB.x);
  p = vec3(p.x * c - p.z * s, p.y, p.x * s + p.z * c);
  vec3 n = vec3(aNormal.x * c - aNormal.z * s, aNormal.y, aNormal.x * s + aNormal.z * c);
  vec3 world = iA.xyz + p;

  if (aSway > 0. && aSway <= 1.001 && m != 9. && m != 10.) {
    float w = length(uWind.xy);
    float amount = aSway * (.025 + w * .5 + uWind.z * .1) * max(iA.w, .4);
    vec2 dir = w > .01 ? uWind.xy / w : vec2(1., 0.);
    float ph = dot(world.xz, vec2(.31, .27)) + uCam.w * (1.1 + w * 1.7) + hash11(iA.x * 7.1 + iA.z) * 6.28;
    float sw = sin(ph) + .5 * sin(ph * 2.17 + 1.3);
    world.xz += dir * (sw * amount + w * .35 * aSway * aSway * max(iA.w, .4));
    world.y -= abs(sw) * amount * .15;
  }
  vWorld = world;
  vN = n;
  vColor = aColor;
  vUV = aUV;
  vMat = aMat;
  vTint = iB.y;
  vSway = aSway;
#ifdef SHADOW
  gl_Position = uLightVP * vec4(world, 1.);
#else
  gl_Position = uVP * vec4(world, 1.);
#endif
}
