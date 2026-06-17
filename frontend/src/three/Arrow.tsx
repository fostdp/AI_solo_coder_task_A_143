import { useMemo, useRef, useEffect, useState } from 'react';
import * as THREE from 'three';
import { useFrame } from '@react-three/fiber';
import { ArrowProps, ARROW_COLOR, TRAIL_START_COLOR, TRAIL_END_COLOR, DEFAULT_PHYSICS_CONFIG } from './types';

export interface ArrowInstanceProps {
  id: number;
  initialPosition: THREE.Vector3;
  initialVelocity: THREE.Vector3;
  onComplete?: (id: number) => void;
  showTrail?: boolean;
  trailLength?: number;
  gravity?: number;
}

function SingleArrow({
  initialPosition,
  initialVelocity,
  onComplete,
  showTrail = true,
  trailLength = 50,
  gravity = DEFAULT_PHYSICS_CONFIG.gravity,
}: Omit<ArrowInstanceProps, 'id'>) {
  const groupRef = useRef<THREE.Group>(null);
  const trailRef = useRef<THREE.Points>(null);
  const positionRef = useRef(initialPosition.clone());
  const velocityRef = useRef(initialVelocity.clone());
  const trailPositionsRef = useRef<THREE.Vector3[]>([]);
  const activeRef = useRef(true);
  const timeRef = useRef(0);

  const arrowGeometries = useMemo(() => {
    const shaftGeometry = new THREE.CylinderGeometry(0.01, 0.01, 0.35, 8);
    shaftGeometry.translate(0, 0.175, 0);

    const headGeometry = new THREE.ConeGeometry(0.02, 0.08, 8);
    headGeometry.translate(0, 0.39, 0);

    const fletchingGeometry = new THREE.ConeGeometry(0.025, 0.05, 4);
    fletchingGeometry.rotateX(Math.PI);
    fletchingGeometry.translate(0, 0.025, 0);

    return { shaftGeometry, headGeometry, fletchingGeometry };
  }, []);

  const arrowMaterial = useMemo(() => {
    return new THREE.MeshStandardMaterial({
      color: ARROW_COLOR,
      roughness: 0.6,
      metalness: 0.3,
    });
  }, []);

  const trailGeometry = useMemo(() => {
    const geometry = new THREE.BufferGeometry();
    const positions = new Float32Array(trailLength * 3);
    const colors = new Float32Array(trailLength * 3);
    geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3));
    return geometry;
  }, [trailLength]);

  const trailMaterial = useMemo(() => {
    return new THREE.PointsMaterial({
      size: 0.03,
      vertexColors: true,
      transparent: true,
      opacity: 0.8,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
  }, []);

  useFrame((_, delta) => {
    if (!activeRef.current || !groupRef.current) return;

    timeRef.current += delta;

    const dt = delta;
    velocityRef.current.y -= gravity * dt;

    positionRef.current.x += velocityRef.current.x * dt;
    positionRef.current.y += velocityRef.current.y * dt;
    positionRef.current.z += velocityRef.current.z * dt;

    groupRef.current.position.copy(positionRef.current);

    const direction = velocityRef.current.clone().normalize();
    const up = new THREE.Vector3(0, 1, 0);
    const quaternion = new THREE.Quaternion();
    quaternion.setFromUnitVectors(up, direction);
    groupRef.current.quaternion.copy(quaternion);

    if (showTrail && trailRef.current) {
      trailPositionsRef.current.push(positionRef.current.clone());
      if (trailPositionsRef.current.length > trailLength) {
        trailPositionsRef.current.shift();
      }

      const positions = trailGeometry.attributes.position.array as Float32Array;
      const colors = trailGeometry.attributes.color.array as Float32Array;
      const startColor = new THREE.Color(TRAIL_START_COLOR);
      const endColor = new THREE.Color(TRAIL_END_COLOR);

      for (let i = 0; i < trailLength; i++) {
        if (i < trailPositionsRef.current.length) {
          const pos = trailPositionsRef.current[i];
          const idx = i * 3;
          positions[idx] = pos.x;
          positions[idx + 1] = pos.y;
          positions[idx + 2] = pos.z;

          const t = i / trailPositionsRef.current.length;
          const color = new THREE.Color().lerpColors(endColor, startColor, t);
          colors[idx] = color.r;
          colors[idx + 1] = color.g;
          colors[idx + 2] = color.b;
        }
      }
      trailGeometry.attributes.position.needsUpdate = true;
      trailGeometry.attributes.color.needsUpdate = true;
    }

    if (positionRef.current.y < -5 || timeRef.current > 10) {
      activeRef.current = false;
      if (onComplete) {
        onComplete(-1);
      }
    }
  });

  return (
    <group ref={groupRef}>
      <mesh geometry={arrowGeometries.shaftGeometry} material={arrowMaterial} />
      <mesh geometry={arrowGeometries.headGeometry} material={arrowMaterial} />
      <mesh geometry={arrowGeometries.fletchingGeometry} material={arrowMaterial} />
      {showTrail && <points ref={trailRef} geometry={trailGeometry} material={trailMaterial} />}
    </group>
  );
}

export function Arrow({
  position,
  rotation = new THREE.Euler(0, 0, 0),
  velocity,
  showTrail = false,
  trailLength = 50,
}: ArrowProps) {
  const groupRef = useRef<THREE.Group>(null);

  const arrowGeometries = useMemo(() => {
    const shaftGeometry = new THREE.CylinderGeometry(0.01, 0.01, 0.35, 8);
    shaftGeometry.translate(0, 0.175, 0);

    const headGeometry = new THREE.ConeGeometry(0.02, 0.08, 8);
    headGeometry.translate(0, 0.39, 0);

    const fletchingGeometry = new THREE.ConeGeometry(0.025, 0.05, 4);
    fletchingGeometry.rotateX(Math.PI);
    fletchingGeometry.translate(0, 0.025, 0);

    return { shaftGeometry, headGeometry, fletchingGeometry };
  }, []);

  const arrowMaterial = useMemo(() => {
    return new THREE.MeshStandardMaterial({
      color: ARROW_COLOR,
      roughness: 0.6,
      metalness: 0.3,
    });
  }, []);

  return (
    <group ref={groupRef} position={position} rotation={rotation}>
      <mesh geometry={arrowGeometries.shaftGeometry} material={arrowMaterial} />
      <mesh geometry={arrowGeometries.headGeometry} material={arrowMaterial} />
      <mesh geometry={arrowGeometries.fletchingGeometry} material={arrowMaterial} />
    </group>
  );
}

export function ArrowManager() {
  const [arrows, setArrows] = useState<ArrowInstanceProps[]>([]);

  const spawnArrow = (
    position: THREE.Vector3,
    velocity: THREE.Vector3,
    showTrail: boolean = true
  ): number => {
    const id = Date.now();
    const newArrow: ArrowInstanceProps = {
      id,
      initialPosition: position.clone(),
      initialVelocity: velocity.clone(),
      showTrail,
    };
    setArrows((prev) => [...prev, newArrow]);
    return id;
  };

  const removeArrow = (id: number) => {
    setArrows((prev) => prev.filter((arrow) => arrow.id !== id));
  };

  const handleArrowComplete = (id: number) => {
    if (id >= 0) {
      removeArrow(id);
    }
  };

  return (
    <>
      {arrows.map((arrow) => (
        <SingleArrow
          key={arrow.id}
          initialPosition={arrow.initialPosition}
          initialVelocity={arrow.initialVelocity}
          showTrail={arrow.showTrail}
          trailLength={arrow.trailLength}
          onComplete={() => handleArrowComplete(arrow.id)}
        />
      ))}
    </>
  );
}

export { SingleArrow };
