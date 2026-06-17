import { useMemo, useRef, useState, useEffect } from 'react';
import * as THREE from 'three';
import { useFrame } from '@react-three/fiber';
import { CrossbowModelProps, CrossbowState, DEFAULT_CROSSBOW_CONFIG, DEFAULT_PHYSICS_CONFIG, WOOD_COLOR, METAL_COLOR, PhysicsConfig, CrossbowConfig } from './types';
import { BowString } from './BowString';
import { Arrow, SingleArrow } from './Arrow';
import { PredictedTrajectory } from './TrajectoryLine';
import { Magazine } from './Magazine';

export function CrossbowModel({
  state,
  config: partialConfig,
  physicsConfig: partialPhysicsConfig,
  onShoot,
}: CrossbowModelProps) {
  const groupRef = useRef<THREE.Group>(null);
  const leftArmRef = useRef<THREE.Mesh>(null);
  const rightArmRef = useRef<THREE.Mesh>(null);
  const camRef = useRef<THREE.Group>(null);
  const pawlRef = useRef<THREE.Mesh>(null);
  const [activeArrows, setActiveArrows] = useState<Array<{ id: number; pos: THREE.Vector3; vel: THREE.Vector3 }>>([]);

  const config: CrossbowConfig = useMemo(() => ({
    ...DEFAULT_CROSSBOW_CONFIG,
    ...partialConfig,
  }), [partialConfig]);

  const physicsConfig: PhysicsConfig = useMemo(() => ({
    ...DEFAULT_PHYSICS_CONFIG,
    ...partialPhysicsConfig,
  }), [partialPhysicsConfig]);

  const woodMaterial = useMemo(() => {
    return new THREE.MeshStandardMaterial({
      color: WOOD_COLOR,
      roughness: 0.8,
      metalness: 0.1,
    });
  }, []);

  const metalMaterial = useMemo(() => {
    return new THREE.MeshStandardMaterial({
      color: METAL_COLOR,
      roughness: 0.4,
      metalness: 0.8,
    });
  }, []);

  const bowArmAngle = useMemo(() => {
    const maxDeformation = physicsConfig.bowArmLength * 0.3;
    const deformation = state.tension * maxDeformation;
    return Math.asin(Math.min(deformation / physicsConfig.bowArmLength, 1));
  }, [state.tension, physicsConfig.bowArmLength]);

  const stringOffset = useMemo(() => {
    const stretchAmount = state.tension * 0.3;
    return stretchAmount * Math.sin(bowArmAngle);
  }, [state.tension, bowArmAngle]);

  const geometries = useMemo(() => {
    const bodyGeometry = new THREE.BoxGeometry(
      config.bodySize.width,
      config.bodySize.height,
      config.bodySize.depth
    );

    const armGeometry = new THREE.BoxGeometry(
      config.bowArm.width,
      config.bowArm.thickness,
      config.bowArm.length
    );

    const camGroup = new THREE.Group();
    const camBase = new THREE.CylinderGeometry(
      config.cam.radius,
      config.cam.radius,
      config.cam.height,
      32
    );
    const camBaseMesh = new THREE.Mesh(camBase, metalMaterial);
    camGroup.add(camBaseMesh);

    for (let i = 0; i < config.cam.teethCount; i++) {
      const angle = (i / config.cam.teethCount) * Math.PI * 2;
      const toothGeometry = new THREE.BoxGeometry(0.015, config.cam.height + 0.002, 0.02);
      const toothMesh = new THREE.Mesh(toothGeometry, metalMaterial);
      toothMesh.position.x = Math.cos(angle) * (config.cam.radius + 0.008);
      toothMesh.position.z = Math.sin(angle) * (config.cam.radius + 0.008);
      toothMesh.rotation.y = -angle;
      camGroup.add(toothMesh);
    }

    const pawlGeometry = new THREE.BoxGeometry(
      config.pawl.width,
      config.pawl.height,
      config.pawl.depth
    );

    return {
      bodyGeometry,
      armGeometry,
      camGroup,
      pawlGeometry,
    };
  }, [config, metalMaterial]);

  useFrame(() => {
    if (leftArmRef.current) {
      leftArmRef.current.rotation.y = bowArmAngle;
    }
    if (rightArmRef.current) {
      rightArmRef.current.rotation.y = -bowArmAngle;
    }
    if (camRef.current) {
      camRef.current.rotation.y = state.camRotation;
    }
    if (pawlRef.current) {
      const targetRotation = state.pawlEngaged ? 0.3 : -0.2;
      pawlRef.current.rotation.x += (targetRotation - pawlRef.current.rotation.x) * 0.2;
    }
  });

  const launchVelocity = useMemo(() => {
    const speed = physicsConfig.initialVelocity * state.tension;
    const angle = physicsConfig.launchAngle;
    return new THREE.Vector3(
      0,
      speed * Math.sin(angle),
      -speed * Math.cos(angle)
    );
  }, [physicsConfig.initialVelocity, physicsConfig.launchAngle, state.tension]);

  const launchPosition = useMemo(() => {
    return new THREE.Vector3(0, 0.02, -0.2 - stringOffset);
  }, [stringOffset]);

  const handleShoot = () => {
    if (state.loaded && state.tension > 0.1) {
      const id = Date.now();
      setActiveArrows((prev) => [
        ...prev,
        {
          id,
          pos: launchPosition.clone(),
          vel: launchVelocity.clone(),
        },
      ]);
      if (onShoot) {
        onShoot();
      }
    }
  };

  const removeArrow = (id: number) => {
    setActiveArrows((prev) => prev.filter((a) => a.id !== id));
  };

  useEffect(() => {
    if (state.loaded === false && state.tension === 0) {
      handleShoot();
    }
  }, [state.loaded, state.tension]);

  return (
    <group ref={groupRef}>
      <mesh
        geometry={geometries.bodyGeometry}
        material={woodMaterial}
        position={[0, 0, -0.1]}
      />

      <mesh
        ref={leftArmRef}
        geometry={geometries.armGeometry}
        material={woodMaterial}
        position={[-0.05, 0, 0.15]}
      />

      <mesh
        ref={rightArmRef}
        geometry={geometries.armGeometry}
        material={woodMaterial}
        position={[0.05, 0, 0.15]}
      />

      <BowString tension={state.tension} />

      <Magazine
        position={state.magazinePosition}
        config={config}
        arrowCount={Math.floor(state.magazinePosition * 10) + 1}
        maxArrows={10}
      />

      <group ref={camRef} position={[0.08, -0.02, 0]}>
        {geometries.camGroup.children.map((child, i) => (
          <primitive key={i} object={child.clone()} />
        ))}
      </group>

      <mesh
        ref={pawlRef}
        geometry={geometries.pawlGeometry}
        material={metalMaterial}
        position={[0.05, -0.01, 0.02]}
        pivot={[0, 0, -0.02]}
      />

      {state.loaded && (
        <Arrow
          position={launchPosition}
          rotation={new THREE.Euler(Math.PI / 2 - physicsConfig.launchAngle, 0, 0)}
        />
      )}

      {state.tension > 0.1 && (
        <PredictedTrajectory
          initialPosition={launchPosition}
          initialVelocity={launchVelocity}
          gravity={physicsConfig.gravity}
        />
      )}

      {activeArrows.map((arrow) => (
        <SingleArrow
          key={arrow.id}
          initialPosition={arrow.pos}
          initialVelocity={arrow.vel}
          showTrail={true}
          onComplete={() => removeArrow(arrow.id)}
        />
      ))}
    </group>
  );
}

export { CrossbowState };
