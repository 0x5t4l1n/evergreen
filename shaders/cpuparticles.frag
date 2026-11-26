in vec4 vCol;
out vec4 oColor;
void main() {
  float d = length(gl_PointCoord - .5);
  float a = vCol.a * sstep(.5, .05, d);
  if (a < .005) discard;
  oColor = vec4(vCol.rgb * (.65 + .5 * uSunDir.w + luma(uAmb.rgb)), a);
}
