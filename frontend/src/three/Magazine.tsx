import { useRef, useMemo, useEffect } from 'react';
import * as THREE from 'three';
import { useFrame } from '@react-three/fiber';
import { CrossbowConfig, DEFAULT_CROSSBOW_CONFIG, METAL_COLOR, ARROW_COLOR } from './types';

interface MagazineProps {
  position: number; // 0~1，箭匣推进位置
  config?: Partial<CrossbowConfig>;
  arrowCount?: number; // 箭匣中剩余箭矢数量
  maxArrows?: number;  // 箭匣最大容量
}

export function Magazine({
  position,
  config: partialConfig,
  arrowCount = 10,
  maxArrows = 10,
}: MagazineProps) {
  const config: CrossbowConfig = useMemo(() => ({
    ...DEFAULT_CROSSBOW_CONFIG,
    ...partialConfig,
  }), [partialConfig]);

  const magGroupRef = useRef<THREE.Group>(null);
  const arrowsInstancedRef = useRef<THREE.InstancedMesh>(null);
  const dummy = useMemo(() => new THREE.Object3D(), []);
  const targetPos = useRef(position);
  const currentPos = useRef(position);

  // 箭尺寸
  const arrowRadius = config.arrow.radius * 0.8;
  const arrowLength = config.arrow.length * 0.6;

  // 箭的排列：3列 x N行
  const cols = 3;
  const rows = Math.ceil(maxArrows / cols);
  const totalInstances = maxArrows;

  // 计算箭在箭匣内的排布偏移
  const arrowSpacingX = (config.magazine.width * 0.7) / cols;
  const arrowSpacingY = (config.magazine.height * 0.7) / Math.max(rows, 1);

  useMemo(() => {
    targetPos.current = position;
  }, [position]);

  useMemo(() => {
    // 初始化箭的位置矩阵
    if (arrowsInstancedRef.current) {
      const instanced = arrowsInstancedRef.current;

      for (let i = 0; i < totalInstances; i++) {
        const col = i % cols;
        const row = Math.floor(i / cols);

        const x = (col - (cols - 1) / 2) * arrowSpacingX;
        const y = (row - (rows - 1) / 2) * arrowSpacingY;
        const z = 0;

        dummy.position.set(x, y, z);
        dummy.rotation.set(Math.PI / 2, 0, 0);
        dummy.scale.set(1, 1, 1);
        dummy.updateMatrix();

        instanced.setMatrixAt(i, dummy.matrix);
        instanced.setColorAt(i, new THREE.Color(ARROW_COLOR));
      }

      instanced.instanceMatrix.needsUpdate = true;
      if (instanced.instanceColor) {
        instanced.instanceColor.needsUpdate = true;
      }
    }
  }, [totalInstances, arrowSpacingX, arrowSpacingY, dummy]);

  // 动画更新：箭匣整体位置 + 箭矢依次减少效果
  useFrame((_, delta) => {
    // 平滑插值箭匣位置
    targetPos.current = position;
    const lerpFactor = Math.min(1, delta * 8);
    currentPos.current += (targetPos.current - currentPos.current) * lerpFactor;

    if (magGroupRef.current) {
      magGroupRef.current.position.z = -0.1 + currentPos.current * 0.15;
    }

    // 更新可见箭矢数量和位置（模拟弹药消耗）
    if (arrowsInstancedRef.current) {
      const instanced = arrowsInstancedRef.current;
      const visibleCount = Math.max(0, Math.min(arrowCount, totalInstances));

      for (let i = 0; i < totalInstances; i++) {
        const col = i % cols;
        const row = Math.floor(i / cols);

        // 只有前面的箭可见，后面的隐藏（缩放到0）
        const isVisible = i < visibleCount;
        const scale = isVisible ? 1.0 : 0.0;

        // 装填动画：箭在箭匣内有微小的前后浮动效果
        const wobble = Math.sin((Date.now() * 0.001) + i * 0.5) * 0.002;
        const zOffset = isVisible ? wobble : 0;

        const x = (col - (cols - 1) / 2) * arrowSpacingX;
        const y = (row - (rows - 1) / 2) * arrowSpacingY;
        const z = zOffset;

        dummy.position.set(x, y, z);
        dummy.rotation.set(Math.PI / 2, 0, 0);
        dummy.scale.set(scale, scale, scale);
        dummy.updateMatrix();

        instanced.setMatrixAt(i, dummy.matrix);
      }

      instanced.instanceMatrix.needsUpdate = true;
    }
  });

  // 箭匣几何体
  const magGeometry = useMemo(() => {
    return new THREE.BoxGeometry(
      config.magazine.width,
      config.magazine.height,
      config.magazine.depth
    );
  }, [config.magazine]);

  const arrowGeometry = useMemo(() => {
    return new THREE.CylinderGeometry(
      arrowRadius * 0.6,  // 尾部略细
      arrowRadius,        // 头部略粗
      arrowLength,
      8  // 8面，比默认32面更高效
    );
  }, [arrowRadius, arrowLength]);

  const arrowMaterial = useMemo(() => {
    return new THREE.MeshStandardMaterial({
      color: ARROW_COLOR,
      roughness: 0.6,
      metalness: 0.1,
    });
  }, []);

  const magMaterial = useMemo(() => {
    return new THREE.MeshStandardMaterial({
      color: METAL_COLOR,
      roughness: 0.4,
      metalness: 0.8,
    });
  }, []);

  return (
    <group ref={magGroupRef} position={[0, -0.08, -0.1]}>
      {/* 箭匣外壳 */}
      <mesh geometry={magGeometry} material={magMaterial} />

      {/* 箭匣内部箭矢（使用InstancedMesh） */}
      <group position={[0, 0, -config.magazine.depth * 0.3]}>
        <instancedMesh
          ref={arrowsInstancedRef}
          args={[arrowGeometry, arrowMaterial, totalInstances]}
          frustumCulled={false}
        />
      </group>
    </group>
  );
}

export default Magazine;
