import * as THREE from "three";
import { EffectComposer } from "three/addons/postprocessing/EffectComposer.js";
import { Pass } from "three/addons/postprocessing/Pass.js";
import { ShaderPass } from "three/addons/postprocessing/ShaderPass.js";
import { UnrealBloomPass } from "three/addons/postprocessing/UnrealBloomPass.js";
import type { SurfaceStyle } from "@neomega/viewer/render";

// 默认场景的画面：方块走线描着色，再叠上辉光、天空和暗角。
// 这些都只属于默认场景，打开存档后不产生任何开销。

export interface BackdropInfo {
  look: [number, number, number];
  fov: number;
  center: [number, number, number];
  radius: number;
  seaLevel: number;
  seaEnd: number;
  twist: [number, number, number];
  lanterns: number;
  glass: number;
}

/** 三个方块面共用的线描 uniform，由 Backdrop 逐帧写入。 */
const line = {
  artOn: { value: 1 },
  artTime: { value: 0 },
  artReveal: { value: 0 },
  artSeaLevel: { value: 64 },
  artOrigin: { value: new THREE.Vector3() },
  // xyz 是环心，w 是半径；倒影在着色器里按当前视线实时求出。
  artRing: { value: new THREE.Vector4() },
};

/** 面内只画四条边，消掉三角形对角线；面本身压暗并写入深度，后面的线被自然遮住。 */
export const lineArtStyle: SurfaceStyle = {
  uniforms: line,
  fragment: /* glsl */ `
    uniform float artOn;
    uniform float artTime;
    uniform float artReveal;
    uniform float artSeaLevel;
    uniform vec3 artOrigin;
    uniform vec4 artRing;
    float ripplePhase(vec2 p) {
      // 与 Go 端 ripplePhase 相同，海面的方块就落在这个函数的整数等值线上。
      float warp = 1.5 * sin(p.x * 0.034 + 1.4 * sin(p.y * 0.021))
        + 0.9 * sin(p.x * 0.071 - p.y * 0.038 + 0.7)
        + 0.5 * sin(p.x * 0.13 + p.y * 0.05);
      float d = length(vec2(p.x, (p.y - 110.0) * 0.8));
      float rings = 2.4 * exp(-pow(d / 120.0, 2.0)) * sin(d * 0.09);
      return p.y / 6.0 + warp + rings;
    }
    float artHash(vec2 p) {
      return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
    }
    void styleSurface(inout vec3 color, inout float alpha, vec3 world, vec2 uv, vec3 faceColor) {
      if (artOn < 0.5) return;
      vec2 face = fract(uv);
      vec2 pixel = max(fwidth(uv), vec2(1e-4));
      vec2 edge = min(face, 1.0 - face) / pixel;
      float wire = 1.0 - smoothstep(0.15, 1.25, min(edge.x, edge.y));
      // 一格只剩几个像素时四条边会糊成实心，按覆盖率压低亮度，让远处细腻而不刺眼。
      wire *= clamp(0.16 / max(pixel.x, pixel.y), 0.1, 1.0) * 0.5 + 0.5;

      float reach = length(world - artOrigin);
      float front = artReveal - reach;
      float shown = smoothstep(0.0, 14.0, front);
      float crest = exp(-front * front / 900.0) * step(0.0, front + 30.0);

      vec3 normal = normalize(cross(dFdx(world), dFdy(world)));
      float sea = 1.0 - smoothstep(0.0, 1.5, abs(world.y - artSeaLevel - 0.5));
      float sunward = exp(-reach / 110.0);
      if (sea > 0.5) {
        // 玻璃只留顶面。方块格线压得很淡，真正的笔触是穿过方块的那条连续涟漪线。
        if (abs(normal.y) < 0.5) { alpha = 0.0; return; }
        float phase = ripplePhase(world.xz);
        float offset = abs(fract(phase + 0.5) - 0.5);
        float stroke = 1.0 - smoothstep(0.0, fwidth(phase) * 1.3, offset - 0.012);
        float moved = artTime * 0.08;
        // 视线在水面反射后打到环所在的平面，命中圆环就是倒影；水面起伏让它碎成波纹。
        vec3 ray = reflect(normalize(world - cameraPosition), vec3(0.0, 1.0, 0.0));
        float travel = (artRing.z - world.z) / ray.z;
        vec3 hit = world + ray * max(travel, 0.0);
        hit.x += 6.0 * sin(phase * 2.2 + moved * 6.0 + world.z * 0.03);
        hit.y += 3.0 * sin(phase * 1.3 - moved * 4.0);
        float ringDistance = abs(length(hit.xy - artRing.xy) - artRing.w);
        float mirrored = (1.0 - smoothstep(6.0, 9.5, ringDistance)) * step(0.0, travel);
        float lower = smoothstep(artRing.y - artRing.w, artSeaLevel + 12.0, hit.y);
        vec2 cell = floor(world.xz);
        float twinkle = pow(0.5 + 0.5 * sin(artTime * (0.8 + artHash(cell) * 2.4) + artHash(cell.yx) * 6.283), 10.0);
        vec3 cold = mix(vec3(0.08, 0.2, 0.7), vec3(0.3, 0.62, 1.0), sunward);
        vec3 warm = mix(vec3(1.0, 0.6, 0.3), vec3(0.6, 0.82, 1.0), lower);
        vec3 ink = mix(cold * (0.16 + 0.8 * sunward), warm * 1.1, mirrored);
        color = ink * (stroke * (0.6 + 0.9 * twinkle) + wire * 0.04);
        color += vec3(1.0, 0.82, 0.6) * crest * stroke * 1.6;
        // 线之间的玻璃近乎不透明的墨色，水下的环只隐约透出来。
        alpha = max(stroke, 0.82) * shown;
      } else {
        // 贴近水线的一段被染成晨光的金色，向上退回海晶灯本来的冷白。
        float rise = smoothstep(artSeaLevel, artSeaLevel + 70.0, world.y);
        // 环从水线的金色升到顶端的冰白；右上角扭转处另加一抹冷蓝，提示它只有一个面。
        vec3 hue = mix(vec3(1.0, 0.58, 0.28), vec3(0.78, 0.9, 1.0), rise);
        float under = world.y < artSeaLevel ? 0.06 : 1.0;
        color = hue * (0.38 + 0.9 * wire) * under * shown;
        color += vec3(1.0, 0.85, 0.65) * crest * wire * under;
      }
    }
  `,
};

/** 把天空垫在线描场景背后。放在辉光之前，地平线那道光也会晕开。 */
const skyPass = {
  uniforms: {
    tDiffuse: { value: null as THREE.Texture | null },
    inverseView: { value: new THREE.Matrix4() },
    inverseProjection: { value: new THREE.Matrix4() },
    sun: { value: new THREE.Vector3() },
    horizon: { value: 0 },
    time: { value: 0 },
    dawn: { value: 0 },
  },
  vertexShader: /* glsl */ `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = vec4(position.xy, 0.0, 1.0);
    }
  `,
  fragmentShader: /* glsl */ `
    uniform sampler2D tDiffuse;
    uniform mat4 inverseView;
    uniform mat4 inverseProjection;
    uniform vec3 sun;
    uniform float horizon;
    uniform float time;
    uniform float dawn;
    varying vec2 vUv;
    float hash(vec2 p) {
      return fract(sin(dot(p, vec2(41.3, 289.1))) * 43758.5453);
    }
    vec3 sky(vec3 ray) {
      float elevation = ray.y - horizon;
      float facing = max(dot(ray, sun), 0.0);
      vec2 level = normalize(sun.xz);
      float side = abs(ray.x * level.y - ray.z * level.x);
      vec3 color = mix(vec3(0.006, 0.014, 0.04), vec3(0.001, 0.003, 0.012), smoothstep(0.0, 0.45, abs(elevation)));
      // 环后的大气：一团冷蓝光晕。
      color += vec3(0.01, 0.045, 0.13) * pow(facing, 60.0) * 1.6;
      // 地平线是一道极细的光，中央偏暖，向两侧变蓝并熄灭。
      float streak = exp(-abs(elevation) * 320.0) * exp(-side * 3.0);
      float glow = exp(-abs(elevation) * 38.0) * exp(-side * 2.4);
      color += mix(vec3(0.15, 0.4, 1.0), vec3(1.0, 0.75, 0.45), exp(-side * 14.0)) * streak * 1.2;
      color += vec3(0.02, 0.07, 0.17) * glow;
      if (elevation > 0.0) {
        vec2 cell = floor(vec2(atan(ray.z, ray.x), ray.y) * 260.0);
        float seed = hash(cell);
        float star = step(0.9965, seed) * (0.5 + 0.5 * sin(time * (0.4 + seed * 2.0) + seed * 90.0));
        color += vec3(0.75, 0.85, 1.0) * star * 0.35 * smoothstep(0.03, 0.25, elevation) * (1.0 - pow(facing, 12.0));
      }
      return color;
    }
    void main() {
      vec4 scene = texture2D(tDiffuse, vUv);
      vec4 view = inverseProjection * vec4(vUv * 2.0 - 1.0, 1.0, 1.0);
      vec3 ray = normalize((inverseView * vec4(view.xyz / view.w, 0.0)).xyz);
      gl_FragColor = vec4(sky(ray) * dawn * (1.0 - clamp(scene.a, 0.0, 1.0)) + scene.rgb, 1.0);
    }
  `,
};

/** 最后一步：胶片肩部、暗角、伽马和颗粒。 */
const finishPass = {
  uniforms: {
    tDiffuse: { value: null as THREE.Texture | null },
    time: { value: 0 },
  },
  vertexShader: skyPass.vertexShader,
  fragmentShader: /* glsl */ `
    uniform sampler2D tDiffuse;
    uniform float time;
    varying vec2 vUv;
    float hash(vec2 p) {
      return fract(sin(dot(p, vec2(41.3, 289.1))) * 43758.5453);
    }
    void main() {
      vec3 color = texture2D(tDiffuse, vUv).rgb;
      color = color * (1.0 + color * 0.16) / (1.0 + color * 0.7);
      vec2 centered = vUv - 0.5;
      color *= 1.0 - smoothstep(0.25, 0.8, length(centered * vec2(1.15, 1.0))) * 0.9;
      color = pow(max(color, 0.0), vec3(1.0 / 2.2));
      color += (hash(vUv * 917.0 + fract(time)) - 0.5) / 150.0;
      gl_FragColor = vec4(color, 1.0);
    }
  `,
};

/** 把线描场景渲染进带透明度的缓冲，天空由合成步骤补在背后。 */
class SceneToBuffer extends Pass {
  draw: (target: THREE.WebGLRenderTarget) => void;
  constructor(draw: (target: THREE.WebGLRenderTarget) => void) {
    super();
    this.draw = draw;
    this.needsSwap = false;
  }
  render(
    _renderer: THREE.WebGLRenderer,
    _write: THREE.WebGLRenderTarget,
    read: THREE.WebGLRenderTarget,
  ): void {
    this.draw(read);
  }
}

export class Backdrop {
  readonly info: BackdropInfo;
  private readonly composer: EffectComposer;
  private readonly sky: ShaderPass;
  private readonly finish: ShaderPass;
  private readonly bloom: UnrealBloomPass;
  private readonly sun: THREE.Vector3;
  private readonly seaEdge: THREE.Vector3;
  private revealStart: number | undefined;
  private started = performance.now();

  constructor(
    renderer: THREE.WebGLRenderer,
    camera: THREE.PerspectiveCamera,
    info: BackdropInfo,
    draw: () => void,
  ) {
    this.info = info;
    line.artSeaLevel.value = info.seaLevel;
    line.artOrigin.value.set(info.center[0], info.seaLevel, info.center[2]);
    line.artReveal.value = 0;
    line.artRing.value.set(...info.center, info.radius);
    this.sun = new THREE.Vector3(...info.center).sub(camera.position).normalize();
    this.seaEdge = new THREE.Vector3(0, info.seaLevel, info.seaEnd * 0.45);
    const size = renderer.getDrawingBufferSize(new THREE.Vector2());
    this.composer = new EffectComposer(
      renderer,
      new THREE.WebGLRenderTarget(size.x, size.y, {
        type: THREE.HalfFloatType,
        samples: 4,
      }),
    );
    this.composer.addPass(new SceneToBuffer((target) => {
      renderer.setRenderTarget(target);
      renderer.setClearColor(0x000000, 0);
      renderer.clear();
      draw();
      renderer.setClearColor(0x000000, 1);
    }));
    this.sky = new ShaderPass(skyPass);
    this.composer.addPass(this.sky);
    this.bloom = new UnrealBloomPass(new THREE.Vector2(size.x, size.y), 0.62, 0.32, 0.42);
    this.composer.addPass(this.bloom);
    this.finish = new ShaderPass(finishPass);
    this.composer.addPass(this.finish);
  }

  /** 场景加载完才开始日出：光从环根部沿海面扩散，把线一路点亮。 */
  beginReveal(now: number): void {
    this.revealStart ??= now;
  }

  setLineArt(value: boolean): void {
    line.artOn.value = value ? 1 : 0;
  }

  resize(renderer: THREE.WebGLRenderer): void {
    const size = renderer.getDrawingBufferSize(new THREE.Vector2());
    this.composer.setPixelRatio(1);
    this.composer.setSize(size.x, size.y);
  }

  render(camera: THREE.PerspectiveCamera, now: number): void {
    const seconds = now / 1000;
    line.artTime.value = seconds;
    const elapsed = this.revealStart === undefined ? 0 : (now - this.revealStart) / 1000;
    // 先快后慢，光到远处时缓缓停下。
    line.artReveal.value = 460 * (1 - Math.exp(-elapsed * 0.62)) + (elapsed > 0 ? elapsed * 6 : 0);
    this.finish.uniforms.time.value = seconds;
    const uniforms = this.sky.uniforms;
    uniforms.inverseView.value.copy(camera.matrixWorld);
    uniforms.inverseProjection.value.copy(camera.projectionMatrixInverse);
    uniforms.sun.value.copy(this.sun);
    uniforms.horizon.value = this.seaEdge.clone().sub(camera.position).normalize().y;
    uniforms.time.value = seconds;
    uniforms.dawn.value = Math.min((now - this.started) / 2600, 1);
    this.composer.render();
  }

  dispose(): void {
    this.bloom.dispose();
    this.sky.dispose();
    this.finish.dispose();
    this.composer.dispose();
  }
}
