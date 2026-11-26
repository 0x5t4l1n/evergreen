in vec2 vUV;
uniform sampler2D uTex;
uniform vec2 uDir;
out vec4 oColor;
void main() {
  vec3 c = texture(uTex, vUV).rgb * .227;
  c += (texture(uTex, vUV + uDir * 1.385).rgb + texture(uTex, vUV - uDir * 1.385).rgb) * .316;
  c += (texture(uTex, vUV + uDir * 3.23).rgb + texture(uTex, vUV - uDir * 3.23).rgb) * .07;
  oColor = vec4(c, 1.);
}
