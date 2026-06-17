import { useMemo, useRef, useEffect } from 'react';
import * as THREE from 'three';
import { TrajectoryLineProps, TRAIL_START_COLOR, TRAIL_END_COLOR } from './types';

export function TrajectoryLine({
  points,
  showImpactMarker = true,
  maxPoints = 100,
}: TrajectoryLineProps) {
  const lineRef = useRef<THREE.Line>(null);
  const markerRef = useRef<THREE.Group>(null);

  const { geometry, material } = useMemo(() => {
    const lineGeometry = new THREE.BufferGeometry();
    const validPoints = points.slice(-maxPoints);
    
    const positions = new Float32Array(validPoints.length * 3);
    const colors = new Float32Array(validPoints.length * 3);
    
    const startColor = new THREE.Color(TRAIL_START_COLOR);
    const endColor = new THREE.Color(TRAIL_END_COLOR);
    
    validPoints.forEach((point, i) => {
      const idx = i * 3;
      positions[idx] = point.x;
      positions[idx + 1] = point.y;
      positions[idx + 2] = point.z;
      
      const t = validPoints.length > 1 ? i / (validPoints.length - 1) : 0;
      const color = new THREE.Color().lerpColors(startColor, endColor, t);
      colors[idx] = color.r;
      colors[idx + 1] = color.g;
      colors[idx + 2] = color.b;
    });
    
    lineGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    lineGeometry.setAttribute('color', new THREE.BufferAttribute(colors, 3));
    
    const lineMaterial = new THREE.LineBasicMaterial({
      vertexColors: true,
      linewidth: 2,
      transparent: true,
      opacity: 0.8,
    });
    
    return { geometry: lineGeometry, material: lineMaterial };
  }, [points, maxPoints]);

  const impactPosition = useMemo(() => {
    if (points.length === 0) return null;
    return points[points.length - 1];
  }, [points]);

  const impactMarkerGeometry = useMemo(() => {
    const ringGeometry = new THREE.RingGeometry(0.08, 0.1, 16);
    const sphereGeometry = new THREE.SphereGeometry(0.03, 8, 8);
    return { ringGeometry, sphereGeometry };
  }, []);

  const impactMaterial = useMemo(() => {
    return new THREE.MeshBasicMaterial({
      color: TRAIL_END_COLOR,
      transparent: true,
      opacity: 0.8,
      side: THREE.DoubleSide,
    });
  }, []);

  return (
    <>
      {points.length > 1 && (
        <line ref={lineRef} geometry={geometry} material={material} />
      )}
      
      {showImpactMarker && impactPosition && (
        <group ref={markerRef} position={impactPosition}>
          <mesh
            geometry={impactMarkerGeometry.ringGeometry}
            material={impactMaterial}
            rotation={[-Math.PI / 2, 0, 0]}
          />
          <mesh
            geometry={impactMarkerGeometry.sphereGeometry}
            material={impactMaterial}
          />
        </group>
      )}
    </>
  );
}

export function PredictedTrajectory({
  initialPosition,
  initialVelocity,
  gravity = 9.8,
  timeStep = 0.05,
  maxTime = 5,
  showImpactMarker = true,
}: {
  initialPosition: THREE.Vector3;
  initialVelocity: THREE.Vector3;
  gravity?: number;
  timeStep?: number;
  maxTime?: number;
  showImpactMarker?: boolean;
}) {
  const predictedPoints = useMemo(() => {
    const points: THREE.Vector3[] = [];
    const pos = initialPosition.clone();
    const vel = initialVelocity.clone();
    
    for (let t = 0; t < maxTime; t += timeStep) {
      points.push(pos.clone());
      
      vel.y -= gravity * timeStep;
      pos.x += vel.x * timeStep;
      pos.y += vel.y * timeStep;
      pos.z += vel.z * timeStep;
      
      if (pos.y < 0) {
        const tToGround = pos.y / (vel.y + gravity * timeStep);
        const groundPos = new THREE.Vector3(
          pos.x - vel.x * tToGround,
          0,
          pos.z - vel.z * tToGround
        );
        points.push(groundPos);
        break;
      }
    }
    
    return points;
  }, [initialPosition, initialVelocity, gravity, timeStep, maxTime]);

  return (
    <TrajectoryLine
      points={predictedPoints}
      showImpactMarker={showImpactMarker}
    />
  );
}
