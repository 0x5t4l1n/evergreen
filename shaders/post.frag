in vec2 vUV;
uniform sampler2D uScene;
uniform sampler2D uBloom;
uniform sampler2D uDepth;
uniform vec4 uPost;  // x bloom strength, y god rays, z dof, w unused
uniform vec4 uSun;   // xy sun uv, z visibility, w unused
out vec4 oColor;

float linDepth(float d) {
  float n = .15, f = 3200.;
  return 2. * n * f / (f + n - (2. * d - 1.) * (f - n));
}

void main() {
  vec2 uv = vUV;
  float under = step(.5, uWater.x);
  uv += under * .004 * vec2(sin(uv.y * 28. + uCam.w * 2.), cos(uv.x * 22. + uCam.w * 1.7));
  vec3 col = texture(uScene, uv).rgb;

  if (uPost.z > .5) { // depth of field
    float d = linDepth(texture(uDepth, uv).r);
    float focus = max(uMisc.x * 0. + uPost.w, 2.);
    float coc = clamp(abs(d - focus) / (focus * .8 + 4.) , 0., 1.) * .012;
    vec3 acc = col;
    for (int i = 0; i < 8; i++) {
      float a = float(i) * .7854 + hash21(uv * 800.) * .5;
      vec2 o = vec2(cos(a), sin(a) * uRes.x * uRes.w) * coc;
      acc += texture(uScene, uv + o).rgb;
      acc += texture(uScene, uv + o * .5).rgb * 0.;
    }
    col = mix(col, acc / 9., sstep(.0, .6, coc * 90.));
  }

  if (uPost.y > .5 && uSun.z > .01) { // god rays
    vec2 delta = (uSun.xy - uv) / 18. * .92;
    vec2 p = uv;
    float w = 1., acc = 0.;
    for (int i = 0; i < 18; i++) {
      p += delta;
      float sky = step(.99999, texture(uDepth, p).r);
      acc += sky * w;
      w *= .93;
    }
    col += uSunCol.rgb * acc * .045 * uSun.z * uSunDir.w * (1. - uParams.z * .7);
  }

  if (uPost.x > 0.) col += texture(uBloom, uv).rgb * uPost.x;

  // grade, contrast, vignette, dither
  col *= uGrade.rgb * 1.3;
  float l = luma(col);
  col = mix(vec3(l), col, uGrade.w);
  col = mix(col, col * col * (3. - 2. * col), .35);
  vec2 q = vUV - .5;
  col *= 1. - dot(q, q) * .8;
  col += (hash21(gl_FragCoord.xy + uCam.w) - .5) / 255.;
  oColor = vec4(col, 1.);
}
