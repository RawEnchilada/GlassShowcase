import * as THREE from "./vendor/three.module.min.js";

const canvas = document.querySelector("#skyCylinder");
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

if (canvas) {
  const cylinderHeight = 420;
  const cylinderBottom = cylinderHeight * -0.5;

  const renderer = new THREE.WebGLRenderer({
    canvas,
    antialias: true,
    alpha: false,
    powerPreference: "high-performance"
  });

  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
  renderer.setClearColor(0x080817, 1);

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(72, 1, 0.1, 600);
  camera.position.set(0, 0, 0);
  camera.rotation.order = "YXZ";
  let currentYaw = THREE.MathUtils.degToRad(window.glassTowerRotationDeg || 0);
  let targetYaw = currentYaw;

  const uniforms = {
    uTime: { value: 0 },
    uPink: { value: new THREE.Color("#ff38d6") },
    uBlue: { value: new THREE.Color("#00e5ff") },
    uWhite: { value: new THREE.Color("#ffffff") },
    uInk: { value: new THREE.Color("#080817") }
  };

  const material = new THREE.ShaderMaterial({
    side: THREE.BackSide,
    depthWrite: false,
    depthTest: false,
    uniforms,
    vertexShader: `
      varying vec2 vUv;

      void main() {
        vUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      precision highp float;

      uniform float uTime;
      uniform vec3 uPink;
      uniform vec3 uBlue;
      uniform vec3 uWhite;
      uniform vec3 uInk;

      varying vec2 vUv;

      void main() {
        float wave = 0.5
          + sin(vUv.x * 18.8495559 + uTime * 0.28) * 0.13
          + sin(vUv.x * 37.6991118 - uTime * 0.16) * 0.035;

        float distanceFromWave = vUv.y - wave;
        float side = smoothstep(-0.18, 0.18, distanceFromWave);
        vec3 color = mix(uPink, uBlue, side);

        float whiteBand = 1.0 - smoothstep(0.0, 0.16, abs(distanceFromWave));
        color = mix(color, uWhite, whiteBand * 0.72);

        float horizonGlow = 1.0 - smoothstep(0.0, 0.36, abs(distanceFromWave));
        color += horizonGlow * 0.16;

        float verticalShade = smoothstep(0.0, 0.22, vUv.y) * (1.0 - smoothstep(0.82, 1.0, vUv.y));
        color = mix(uInk, color, 0.62 + verticalShade * 0.38);

        gl_FragColor = vec4(color, 1.0);
      }
    `
  });

  const cylinder = new THREE.Mesh(
    new THREE.CylinderGeometry(70, 70, cylinderHeight, 192, 1, true),
    material
  );
  cylinder.rotation.y = Math.PI * 0.18;
  scene.add(cylinder);

  const glassUniforms = {
    uTime: { value: 0 }
  };

  const glassMaterial = new THREE.ShaderMaterial({
      transparent: true,
      depthWrite: false,
      depthTest: true,
      uniforms: glassUniforms,
      vertexShader: `
        varying vec2 vUv;
        uniform float uTime;

        void main() {
          vUv = uv;
          vec3 warped = position;
          warped.z += sin((uv.y * 9.0) + uTime * 0.55) * 0.018;
          warped.z += sin((uv.x * 15.0) - uTime * 0.38) * 0.01;
          gl_Position = projectionMatrix * modelViewMatrix * vec4(warped, 1.0);
        }
      `,
      fragmentShader: `
        precision highp float;

        uniform float uTime;
        varying vec2 vUv;

        void main() {
          float ripple = sin(vUv.y * 19.0 + uTime * 0.72) * 0.5 + 0.5;
          ripple += sin((vUv.x + vUv.y) * 13.0 - uTime * 0.45) * 0.22;

          float edgeX = smoothstep(0.0, 0.055, vUv.x) * (1.0 - smoothstep(0.945, 1.0, vUv.x));
          float edgeY = smoothstep(0.0, 0.055, vUv.y) * (1.0 - smoothstep(0.945, 1.0, vUv.y));
          float edge = 1.0 - edgeX * edgeY;

          float vertical = 1.0 - smoothstep(0.0, 0.045, abs(vUv.x + sin(vUv.y * 10.0 + uTime * 0.3) * 0.025 - 0.55));
          float horizontal = 1.0 - smoothstep(0.0, 0.055, abs(vUv.y + sin(vUv.x * 8.0 - uTime * 0.22) * 0.018 - 0.68));

          vec3 color = vec3(0.82, 0.98, 1.0);
          color = mix(color, vec3(1.0, 0.72, 0.96), ripple * 0.18);
          color += vertical * vec3(0.12, 0.2, 0.23);
          color += horizontal * vec3(0.09, 0.16, 0.18);
          color += edge * vec3(0.34);

          float alpha = 0.14 + ripple * 0.08 + vertical * 0.08 + horizontal * 0.06 + edge * 0.22;
          gl_FragColor = vec4(color, alpha);
        }
      `
  });

  const edgeMaterial = new THREE.LineBasicMaterial({
    color: 0xffffff,
    transparent: true,
    opacity: 0.32,
    depthTest: true
  });

  function wallGeometry(width, height, topScale = 1, skew = 0) {
    const half = width * 0.5;
    const topHalf = half * topScale;
    const positions = new Float32Array([
      -half, 0, 0,
      half, 0, 0,
      topHalf + skew, height, 0,
      -topHalf + skew, height, 0
    ]);
    const uvs = new Float32Array([
      0, 0,
      1, 0,
      1, 1,
      0, 1
    ]);
    const indices = [0, 1, 2, 0, 2, 3];
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    geometry.setAttribute("uv", new THREE.BufferAttribute(uvs, 2));
    geometry.setIndex(indices);
    geometry.computeVertexNormals();
    return geometry;
  }

  const wallRig = new THREE.Group();
  scene.add(wallRig);

  const wallSpecs = [
    { angle: -0.92, radius: 42, width: 10, height: 252, topScale: 0.82, skew: -1.8 },
    { angle: -0.66, radius: 58, width: 20, height: 292, topScale: 0.74, skew: 2.8 },
    { angle: -0.48, radius: 47, width: 8, height: 232, topScale: 1.18, skew: 1.2 },
    { angle: -0.28, radius: 61, width: 17, height: 278, topScale: 1.28, skew: -2.0 },
    { angle: -0.13, radius: 54, width: 12, height: 276, topScale: 0.72, skew: 0.5 },
    { angle: 0.31, radius: 45, width: 7, height: 246, topScale: 1.05, skew: -0.9 },
    { angle: 0.52, radius: 62, width: 23, height: 306, topScale: 0.68, skew: -3.2 },
    { angle: 0.78, radius: 50, width: 11, height: 266, topScale: 0.9, skew: 1.6 },
    { angle: 1.12, radius: 60, width: 18, height: 286, topScale: 1.16, skew: 2.1 },
    { angle: 1.35, radius: 56, width: 8, height: 238, topScale: 1.25, skew: -1.3 },
    { angle: 1.72, radius: 64, width: 21, height: 314, topScale: 0.8, skew: 2.6 },
    { angle: 2.05, radius: 52, width: 13, height: 284, topScale: 0.76, skew: 2.2 },
    { angle: 2.34, radius: 59, width: 16, height: 268, topScale: 1.34, skew: -2.5 },
    { angle: 2.72, radius: 44, width: 9, height: 256, topScale: 1.12, skew: -1.1 },
    { angle: 3.08, radius: 63, width: 24, height: 300, topScale: 0.7, skew: 3.0 },
    { angle: 3.38, radius: 58, width: 12, height: 296, topScale: 0.86, skew: 0.8 },
    { angle: 3.68, radius: 61, width: 19, height: 274, topScale: 1.2, skew: -2.8 },
    { angle: 4.02, radius: 46, width: 7, height: 228, topScale: 1.32, skew: 1.4 },
    { angle: 4.34, radius: 60, width: 22, height: 304, topScale: 0.76, skew: 2.2 },
    { angle: 4.68, radius: 55, width: 14, height: 270, topScale: 0.68, skew: -2.4 },
    { angle: 4.98, radius: 63, width: 18, height: 282, topScale: 1.26, skew: -1.8 },
    { angle: 5.42, radius: 49, width: 9, height: 242, topScale: 1.08, skew: 1.0 },
    { angle: 5.72, radius: 62, width: 21, height: 294, topScale: 0.82, skew: -3.1 },
    { angle: 6.05, radius: 57, width: 15, height: 264, topScale: 1.18, skew: 1.9 }
  ];

  const glassPanes = wallSpecs.map((spec) => {
    const geometry = wallGeometry(spec.width, spec.height, spec.topScale, spec.skew);
    const mesh = new THREE.Mesh(geometry, glassMaterial);
    mesh.position.set(
      Math.sin(spec.angle) * spec.radius,
      cylinderBottom,
      Math.cos(spec.angle) * spec.radius
    );
    mesh.rotation.y = spec.angle + Math.PI;
    mesh.renderOrder = 2;
    wallRig.add(mesh);

    const edges = new THREE.LineSegments(new THREE.EdgesGeometry(geometry), edgeMaterial);
    edges.position.copy(mesh.position);
    edges.rotation.copy(mesh.rotation);
    edges.renderOrder = 3;
    wallRig.add(edges);

    return mesh;
  });

  function resize() {
    const width = Math.max(1, Math.round(window.innerWidth));
    const height = Math.max(1, Math.round(window.innerHeight));
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    camera.updateProjectionMatrix();

  }

  function render(time = 0) {
    uniforms.uTime.value = time * 0.001;
    glassUniforms.uTime.value = uniforms.uTime.value;
    currentYaw += (targetYaw - currentYaw) * (reducedMotion ? 1 : 0.08);
    camera.rotation.y = currentYaw;
    cylinder.rotation.y = Math.PI * 0.18;
    renderer.render(scene, camera);

    if (!reducedMotion) {
      window.requestAnimationFrame(render);
    }
  }

  window.addEventListener("resize", resize);
  window.addEventListener("exhibitionrotate", (event) => {
    targetYaw = THREE.MathUtils.degToRad(event.detail.degrees || 0);
  });

  window.setExhibitionRotation = (degrees) => {
    targetYaw = THREE.MathUtils.degToRad(degrees || 0);
  };

  resize();
  render();
}
