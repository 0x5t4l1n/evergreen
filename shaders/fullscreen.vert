out vec2 vNdc;
out vec2 vUV;
void main() {
  vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  vNdc = p * 2. - 1.;
  vUV = p;
  gl_Position = vec4(vNdc, 1., 1.);
}
