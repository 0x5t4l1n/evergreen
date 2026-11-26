layout(location = 0) in vec3 aPos;
layout(location = 1) in float aSize;
layout(location = 2) in vec4 aCol;
out vec4 vCol;
void main() {
  vec4 clip = uVP * vec4(aPos, 1.);
  float dist = max(clip.w, .1);
  gl_PointSize = clamp(aSize * uRes.y * .5 / (uCamU.w * dist), 1., 220.);
  vCol = aCol;
  gl_Position = clip;
}
