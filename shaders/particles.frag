in vec4 vCol;
in float vAng;
uniform int uKind;
uniform int uLines;
out vec4 oColor;
void main() {
  float a = vCol.a;
  if (uLines == 0) {
    vec2 q = gl_PointCoord - .5;
    if (uKind == 4) {
      float c = cos(vAng), s = sin(vAng);
      q = vec2(q.x * c - q.y * s, q.x * s + q.y * c);
      float d = length(q * vec2(1., 2.1));
      a *= sstep(.5, .3, d);
    } else {
      a *= sstep(.5, .0, length(q));
    }
  }
  if (a < .01) discard;
  oColor = vec4(vCol.rgb, a);
}
