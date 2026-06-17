/**
 * repeating_crossbow_3d.js
 *
 * 诸葛连弩三维渲染模块
 * 功能：弩身、弩臂、弓弦、箭匣、凸轮机构的3D模型渲染
 *      弓弦拉伸动画、箭匣装填动画、箭矢发射动画
 *      弹道轨迹可视化
 *
 * 依赖：Three.js, @react-three/fiber, @react-three/drei
 */

import React, { useMemo, useRef, useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import * as THREE from 'three';
import { useFrame } from '@react-three/fiber';
import { OrbitControls } from '@react-three/drei';

// ============ 常量定义 ============
const WOOD_COLOR = '#8B4513';
const METAL_COLOR = '#708090';
const ARROW_COLOR = '#4A4A4A';
const STRING_COLOR = '#333333';
const STRING_TENSION_COLOR = '#FF4444';
const TRAJECTORY_COLOR = '#FFAA00';

// ============ 默认配置 ============
const DEFAULT_CONFIG = {
  bodySize: { width: 0.08, height: 0.04, depth: 0.9 },
  bowArm: { length: 0.6, width: 0.04, thickness: 0.015, maxDeformation: 0.02 },
  bowString: { length0: 0.85, radius: 0.002, preTension: 200.0 },
  cam: { radius: 0.04, height: 0.06, teethCount: 24 },
  pawl: { width: 0.02, height: 0.015, depth: 0.04 },
  arrow: { radius: 0.004, length: 0.5, tipMass: 0.015 },
  magazine: { width: 0.12, height: 0.15, depth: 0.1, capacity: 10 },
};

// ============ 连弩状态接口 ============
export interface CrossbowState {
  tension: number;          // 弓弦张力 [N]
  deformation: number;      // 弩臂变形 [m]
  magazinePosition: number; // 箭匣位置 [0~1]
  fireRate: number;         // 射速 [发/分钟]
  camRotation: number;      // 凸轮转角 [rad]
  pawlEngaged: boolean;     // 棘爪是否啮合
  stringFatigue: number;    // 弓弦疲劳 [0~1]
  shotsFired: number;       // 已发射次数
  isFiring: boolean;        // 是否正在击发
}

export const DEFAULT_STATE: CrossbowState = {
  tension: 200,
  deformation: 0,
  magazinePosition: 0,
  fireRate: 8,
  camRotation: 0,
  pawlEngaged: false,
  stringFatigue: 0,
  shotsFired: 0,
  isFiring: false,
};

// ============ 对外暴露方法 ============
export interface Crossbow3DHandle {
  fireArrow: (initialVelocity: THREE.Vector3, launchAngle: number) => void;
  updateState: (state: Partial<CrossbowState>) => void;
  getState: () => CrossbowState;
}

// ============ 组件：弩臂 ============
function BowArm({ side, angle, config }) {
  const ref = useRef<THREE.Mesh>(null);
  const restAngle = side === 'left' ? 0.3 : -0.3;

  useFrame(() => {
    if (ref.current) {
      const targetAngle = restAngle + (side === 'left' ? angle : -angle);
      ref.current.rotation.y += (targetAngle - ref.current.rotation.y) * 0.2;
    }
  });

  return (
    <mesh
      ref={ref}
      position={[side === 'left' ? 0.05 : -0.05, 0, 0.35]}
    >
      <boxGeometry args={[config.width, config.thickness, config.length]} />
      <meshStandardMaterial color={WOOD_COLOR} roughness={0.7} />
    </mesh>
  );
}

// ============ 组件：弓弦 ============
function BowString({ tension, config }) {
  const curveRef = useRef<THREE.TubeGeometry>(null);
  const materialRef = useRef<THREE.MeshStandardMaterial>(null);
  const vibrationRef = useRef(0);
  const vibrationVelRef = useRef(0);

  const stringCurve = useMemo(() => {
    const maxTension = 1500;
    const tensionRatio = Math.min(tension / maxTension, 1);
    const zOffset = -tension * 0.0003; // 张力越大，弓弦越向后

    // 5点CatmullRom曲线：左锚点→左控制点→中心点→右控制点→右锚点
    const points = [
      new THREE.Vector3(0.35, 0, 0.35),
      new THREE.Vector3(0.2, 0, 0.3 + zOffset * 0.3),
      new THREE.Vector3(0, 0, 0.3 + zOffset + vibrationRef.current * 0.01),
      new THREE.Vector3(-0.2, 0, 0.3 + zOffset * 0.3),
      new THREE.Vector3(-0.35, 0, 0.35),
    ];

    return new THREE.CatmullRomCurve3(points);
  }, [tension, vibrationRef.current]);

  useFrame((_, delta) => {
    // 弓弦颜色：张力越大越红
    if (materialRef.current) {
      const t = Math.min(tension / 1500, 1);
      const color = new THREE.Color(STRING_COLOR).lerp(
        new THREE.Color(STRING_TENSION_COLOR),
        t
      );
      materialRef.current.color.copy(color);
    }

    // 释放时的振动衰减
    if (Math.abs(vibrationVelRef.current) > 0.001 || Math.abs(vibrationRef.current) > 0.001) {
      const damping = 0.95;
      const k = 50; // 振动恢复力系数
      const accel = -k * vibrationRef.current - 0.5 * vibrationVelRef.current;

      vibrationVelRef.current += accel * delta;
      vibrationVelRef.current *= damping;
      vibrationRef.current += vibrationVelRef.current * delta;
    }
  });

  // 暴露振动触发方法（击发时调用）
  useEffect(() => {
    if (tension < 200 && tension > 100) {
      // 检测到快速释放，触发振动
      vibrationVelRef.current = 2.0;
    }
  }, [tension]);

  return (
    <mesh>
      <tubeGeometry
        ref={curveRef}
        args={[stringCurve, 32, config.radius, 8, false]}
      />
      <meshStandardMaterial
        ref={materialRef}
        color={STRING_COLOR}
        roughness={0.9}
        metalness={0.1}
      />
    </mesh>
  );
}

// ============ 组件：箭匣（GPU实例化箭矢） ============
function Magazine({ position, arrowCount, config }) {
  const groupRef = useRef<THREE.Group>(null);
  const instancedRef = useRef<THREE.InstancedMesh>(null);
  const dummy = useMemo(() => new THREE.Object3D(), []);
  const targetPos = useRef(position);
  const currentPos = useRef(position);

  const cols = 3;
  const rows = Math.ceil(config.capacity / cols);
  const totalInstances = config.capacity;

  const arrowSpacingX = (config.width * 0.7) / cols;
  const arrowSpacingY = (config.height * 0.7) / Math.max(rows, 1);

  const arrowRadius = config.arrowRadius * 0.8;
  const arrowLength = config.arrowLength * 0.6;

  // 初始化实例矩阵
  useMemo(() => {
    if (instancedRef.current) {
      for (let i = 0; i < totalInstances; i++) {
        const col = i % cols;
        const row = Math.floor(i / cols);
        const x = (col - (cols - 1) / 2) * arrowSpacingX;
        const y = (row - (rows - 1) / 2) * arrowSpacingY;

        dummy.position.set(x, y, 0);
        dummy.rotation.set(Math.PI / 2, 0, 0);
        dummy.scale.set(1, 1, 1);
        dummy.updateMatrix();

        instancedRef.current.setMatrixAt(i, dummy.matrix);
        instancedRef.current.setColorAt(i, new THREE.Color(ARROW_COLOR));
      }
      instancedRef.current.instanceMatrix.needsUpdate = true;
    }
  }, [totalInstances]);

  // 动画更新
  useFrame((_, delta) => {
    // 箭匣位置平滑插值
    targetPos.current = position;
    const lerpFactor = Math.min(1, delta * 8);
    currentPos.current += (targetPos.current - currentPos.current) * lerpFactor;

    if (groupRef.current) {
      groupRef.current.position.z = -0.1 + currentPos.current * 0.15;
    }

    // 更新箭矢实例（弹药消耗效果）
    if (instancedRef.current) {
      const visibleCount = Math.max(0, Math.min(arrowCount, totalInstances));
      for (let i = 0; i < totalInstances; i++) {
        const col = i % cols;
        const row = Math.floor(i / cols);
        const isVisible = i < visibleCount;
        const scale = isVisible ? 1.0 : 0.0;
        const wobble = Math.sin((Date.now() * 0.001) + i * 0.5) * 0.002;
        const zOffset = isVisible ? wobble : 0;

        const x = (col - (cols - 1) / 2) * arrowSpacingX;
        const y = (row - (rows - 1) / 2) * arrowSpacingY;

        dummy.position.set(x, y, zOffset);
        dummy.rotation.set(Math.PI / 2, 0, 0);
        dummy.scale.set(scale, scale, scale);
        dummy.updateMatrix();

        instancedRef.current.setMatrixAt(i, dummy.matrix);
      }
      instancedRef.current.instanceMatrix.needsUpdate = true;
    }
  });

  return (
    <group ref={groupRef} position={[0, -0.08, -0.1]}>
      {/* 箭匣外壳 */}
      <mesh>
        <boxGeometry args={[config.width, config.height, config.depth]} />
        <meshStandardMaterial color={METAL_COLOR} roughness={0.4} metalness={0.8} />
      </mesh>

      {/* 箭矢（InstancedMesh） */}
      <group position={[0, 0, -config.depth * 0.3]}>
        <instancedMesh
          ref={instancedRef}
          args={[
            new THREE.CylinderGeometry(arrowRadius * 0.6, arrowRadius, arrowLength, 8),
            new THREE.MeshStandardMaterial({
              color: ARROW_COLOR,
              roughness: 0.6,
              metalness: 0.1,
            }),
            totalInstances,
          ]}
          frustumCulled={false}
        />
      </group>
    </group>
  );
}

// ============ 组件：凸轮机构 ============
function CamMechanism({ rotation, pawlEngaged, config }) {
  const camRef = useRef<THREE.Group>(null);
  const pawlRef = useRef<THREE.Mesh>(null);

  // 构建凸轮组（圆柱+齿）
  const camGroup = useMemo(() => {
    const group = new THREE.Group();

    const baseGeom = new THREE.CylinderGeometry(
      config.cam.radius,
      config.cam.radius,
      config.cam.height,
      32
    );
    const baseMesh = new THREE.Mesh(
      baseGeom,
      new THREE.MeshStandardMaterial({ color: METAL_COLOR, metalness: 0.9 })
    );
    group.add(baseMesh);

    // 添加齿
    for (let i = 0; i < config.cam.teethCount; i++) {
      const angle = (i / config.cam.teethCount) * Math.PI * 2;
      const toothGeom = new THREE.BoxGeometry(0.015, config.cam.height + 0.002, 0.02);
      const toothMesh = new THREE.Mesh(
        toothGeom,
        new THREE.MeshStandardMaterial({ color: METAL_COLOR, metalness: 0.9 })
      );
      toothMesh.position.x = Math.cos(angle) * (config.cam.radius + 0.008);
      toothMesh.position.z = Math.sin(angle) * (config.cam.radius + 0.008);
      toothMesh.rotation.y = -angle;
      group.add(toothMesh);
    }

    return group;
  }, [config.cam]);

  useFrame(() => {
    if (camRef.current) {
      camRef.current.rotation.y = rotation;
    }
    if (pawlRef.current) {
      const target = pawlEngaged ? 0.3 : -0.2;
      pawlRef.current.rotation.x += (target - pawlRef.current.rotation.x) * 0.2;
    }
  });

  return (
    <group position={[0, 0, -0.2]}>
      <primitive object={camGroup} ref={camRef} />

      {/* 棘爪 */}
      <mesh ref={pawlRef} position={[0.06, 0, -0.2]}>
        <boxGeometry args={[config.pawl.width, config.pawl.height, config.pawl.depth]} />
        <meshStandardMaterial color={METAL_COLOR} metalness={0.8} />
      </mesh>
    </group>
  );
}

// ============ 组件：飞行中的箭矢 ============
function FlyingArrow({ position, velocity, onComplete }) {
  const ref = useRef<THREE.Group>(null);
  const trailRef = useRef<THREE.Points>(null);
  const startTime = useRef(Date.now());
  const trailPositions = useRef<Float32Array>(new Float32Array(100 * 3));
  const trailColors = useRef<Float32Array>(new Float32Array(100 * 3));
  const trailIndex = useRef(0);

  useFrame((_, delta) => {
    if (!ref.current) return;

    // 更新位置（欧拉积分）
    const gravity = 9.81;
    const airDensity = 1.225;
    const dragCoeff = 0.47;
    const arrowRadius = 0.004;
    const mass = 0.05;

    const speed = velocity.length();
    const dragForce = 0.5 * airDensity * dragCoeff * Math.PI * arrowRadius * arrowRadius * speed * speed;
    const dragAccel = dragForce / mass;

    if (speed > 0.01) {
      const dragDir = velocity.clone().normalize().multiplyScalar(-dragAccel);
      velocity.add(dragDir.multiplyScalar(delta));
    }

    velocity.y -= gravity * delta;
    position.add(velocity.clone().multiplyScalar(delta));

    ref.current.position.copy(position);

    // 箭矢朝向速度方向
    if (speed > 0.1) {
      ref.current.lookAt(position.clone().add(velocity));
      ref.current.rotateX(Math.PI / 2);
    }

    // 更新轨迹粒子
    const idx = trailIndex.current % 100;
    trailPositions.current[idx * 3] = position.x;
    trailPositions.current[idx * 3 + 1] = position.y;
    trailPositions.current[idx * 3 + 2] = position.z;

    // 颜色渐变（橙→红→透明）
    const age = (Date.now() - startTime.current) / 1000;
    const color = new THREE.Color(TRAJECTORY_COLOR).lerp(
      new THREE.Color(0xFF0000),
      Math.min(age / 3, 1)
    );
    trailColors.current[idx * 3] = color.r;
    trailColors.current[idx * 3 + 1] = color.g;
    trailColors.current[idx * 3 + 2] = color.b;

    trailIndex.current++;

    if (trailRef.current) {
      trailRef.current.geometry.attributes.position.needsUpdate = true;
      trailRef.current.geometry.attributes.color.needsUpdate = true;
    }

    // 落地检测
    if (position.y < 0 || age > 10) {
      onComplete?.();
    }
  });

  const trailGeometry = useMemo(() => {
    const geom = new THREE.BufferGeometry();
    geom.setAttribute('position', new THREE.BufferAttribute(trailPositions.current, 3));
    geom.setAttribute('color', new THREE.BufferAttribute(trailColors.current, 3));
    return geom;
  }, []);

  return (
    <group>
      {/* 箭矢主体 */}
      <group ref={ref}>
        <mesh>
          <cylinderGeometry args={[0.004, 0.004, 0.5, 8]} />
          <meshStandardMaterial color={ARROW_COLOR} />
        </mesh>
        <mesh position={[0, 0.26, 0]}>
          <coneGeometry args={[0.006, 0.05, 8]} />
          <meshStandardMaterial color={METAL_COLOR} metalness={0.9} />
        </mesh>
        <mesh position={[0, -0.26, 0]}>
          <coneGeometry args={[0.008, 0.02, 4]} />
          <meshStandardMaterial color="#FF0000" />
        </mesh>
      </group>

      {/* 轨迹粒子 */}
      <points ref={trailRef} geometry={trailGeometry}>
        <pointsMaterial
          size={0.02}
          vertexColors
          transparent
          opacity={0.8}
          blending={THREE.AdditiveBlending}
        />
      </points>
    </group>
  );
}

// ============ 主组件：诸葛连弩3D模型 ============
export const RepeatingCrossbow3D = forwardRef<Crossbow3DHandle, {
  initialState?: Partial<CrossbowState>;
  config?: Partial<typeof DEFAULT_CONFIG>;
}>(({ initialState, config: partialConfig }, ref) => {
  const config = useMemo(() => ({
    ...DEFAULT_CONFIG,
    ...partialConfig,
    bowArm: { ...DEFAULT_CONFIG.bowArm, ...(partialConfig?.bowArm || {}) },
    cam: { ...DEFAULT_CONFIG.cam, ...(partialConfig?.cam || {}) },
  }), [partialConfig]);

  const [state, setState] = useState<CrossbowState>({
    ...DEFAULT_STATE,
    ...initialState,
  });

  const [flyingArrows, setFlyingArrows] = useState<Array<{
    id: number;
    position: THREE.Vector3;
    velocity: THREE.Vector3;
  }>>([]);

  const stateRef = useRef(state);
  const arrowIdCounter = useRef(0);

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  // 弩臂弯角计算
  const bowArmAngle = useMemo(() => {
    const maxAngle = Math.asin(
      (config.bowArm.maxDeformation / config.bowArm.length) * 2
    );
    const tensionRatio = Math.min(state.tension / 1500, 1);
    return maxAngle * tensionRatio;
  }, [state.tension, config.bowArm]);

  // 对外暴露方法
  useImperativeHandle(ref, () => ({
    fireArrow: (initialVelocity: THREE.Vector3, launchAngle: number) => {
      const pos = new THREE.Vector3(0, 0, 0);
      const vel = initialVelocity.clone();
      const angle = launchAngle * Math.PI / 180;
      vel.applyAxisAngle(new THREE.Vector3(1, 0, 0), angle);

      const newArrow = {
        id: arrowIdCounter.current++,
        position: pos,
        velocity: vel,
      };

      setFlyingArrows(prev => [...prev, newArrow]);
      setState(prev => ({ ...prev, isFiring: true, shotsFired: prev.shotsFired + 1 }));

      setTimeout(() => {
        setState(prev => ({ ...prev, isFiring: false }));
      }, 100);
    },

    updateState: (newState: Partial<CrossbowState>) => {
      setState(prev => ({ ...prev, ...newState }));
    },

    getState: () => stateRef.current,
  }));

  // 移除完成的箭矢
  const removeArrow = (id: number) => {
    setFlyingArrows(prev => prev.filter(a => a.id !== id));
  };

  return (
    <>
      {/* 场景光照 */}
      <ambientLight intensity={0.5} />
      <directionalLight position={[5, 10, 5]} intensity={1} castShadow />
      <directionalLight position={[-5, 5, -5]} intensity={0.3} />

      {/* 地面网格 */}
      <gridHelper args={[10, 20, 0x444444, 0x222222]} />

      {/* 连弩主体 */}
      <group rotation={[0, Math.PI / 2, 0]}>
        {/* 弩身 */}
        <mesh position={[0, 0, 0]}>
          <boxGeometry args={[config.bodySize.width, config.bodySize.height, config.bodySize.depth]} />
          <meshStandardMaterial color={WOOD_COLOR} roughness={0.7} />
        </mesh>

        {/* 左右弩臂 */}
        <BowArm side="left" angle={bowArmAngle} config={config.bowArm} />
        <BowArm side="right" angle={bowArmAngle} config={config.bowArm} />

        {/* 弓弦 */}
        <BowString tension={state.tension} config={config.bowString} />

        {/* 箭匣 */}
        <Magazine
          position={state.magazinePosition}
          arrowCount={Math.floor(state.magazinePosition * config.magazine.capacity) + 1}
          config={{
            ...config.magazine,
            arrowRadius: config.arrow.radius,
            arrowLength: config.arrow.length,
          }}
        />

        {/* 凸轮机构 */}
        <CamMechanism
          rotation={state.camRotation}
          pawlEngaged={state.pawlEngaged}
          config={config}
        />

        {/* 待发射箭 */}
        <mesh position={[0, 0, 0.3]} rotation={[Math.PI / 2, 0, 0]}>
          <cylinderGeometry args={[0.004, 0.004, 0.5, 8]} />
          <meshStandardMaterial color={ARROW_COLOR} />
        </mesh>
      </group>

      {/* 飞行中的箭矢 */}
      {flyingArrows.map(arrow => (
        <FlyingArrow
          key={arrow.id}
          position={arrow.position.clone()}
          velocity={arrow.velocity.clone()}
          onComplete={() => removeArrow(arrow.id)}
        />
      ))}

      {/* 相机控制 */}
      <OrbitControls
        enableDamping
        dampingFactor={0.05}
        minDistance={0.5}
        maxDistance={10}
        target={[0, 0, 0]}
      />
    </>
  );
});

RepeatingCrossbow3D.displayName = 'RepeatingCrossbow3D';

export default RepeatingCrossbow3D;
