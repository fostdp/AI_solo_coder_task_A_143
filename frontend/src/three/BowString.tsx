import { useMemo, useRef, useEffect } from 'react';
import * as THREE from 'three';
import { useFrame } from '@react-three/fiber';
import { BowStringProps, STRING_COLOR, STRING_TENSION_COLOR } from './types';

export function BowString({
  tension,
  width = 0.008,
  vibrationAmplitude = 0.02,
  vibrationDamping = 0.95,
}: BowStringProps) {
  const meshRef = useRef<THREE.Mesh>(null);
  const vibrationRef = useRef(0);
  const lastTensionRef = useRef(tension);
  const stringColorRef = useRef(new THREE.Color(STRING_COLOR));

  useEffect(() => {
    if (lastTensionRef.current > 0.3 && tension === 0) {
      vibrationRef.current = vibrationAmplitude;
    }
    lastTensionRef.current = tension;
  }, [tension, vibrationAmplitude]);

  useFrame(() => {
    if (vibrationRef.current > 0.0001) {
      vibrationRef.current *= vibrationDamping;
    } else {
      vibrationRef.current = 0;
    }
  });

  const { geometry, material } = useMemo(() => {
    const bowWidth = 0.9;
    const stretchAmount = tension * 0.3;
    
    const vibrationOffset = Math.sin(Date.now() * 0.01) * vibrationRef.current;

    const leftAnchor = new THREE.Vector3(-bowWidth / 2, 0, 0.2);
    const rightAnchor = new THREE.Vector3(bowWidth / 2, 0, 0.2);
    const centerPoint = new THREE.Vector3(
      0 + vibrationOffset * 0.5,
      0,
      -stretchAmount + vibrationOffset * 0.3
    );

    const leftControl = new THREE.Vector3(
      -bowWidth / 4,
      0,
      -stretchAmount * 0.3 + vibrationOffset * 0.4
    );
    const rightControl = new THREE.Vector3(
      bowWidth / 4,
      0,
      -stretchAmount * 0.3 + vibrationOffset * 0.4
    );

    const curve = new THREE.CatmullRomCurve3([
      leftAnchor,
      leftControl,
      centerPoint,
      rightControl,
      rightAnchor,
    ]);

    const tubeGeometry = new THREE.TubeGeometry(curve, 32, width, 8, false);

    const color = new THREE.Color();
    const baseColor = new THREE.Color(STRING_COLOR);
    const tensionColor = new THREE.Color(STRING_TENSION_COLOR);
    color.lerpColors(baseColor, tensionColor, tension);
    stringColorRef.current.copy(color);

    const tubeMaterial = new THREE.MeshStandardMaterial({
      color: color,
      roughness: 0.7,
      metalness: 0.1,
      emissive: color,
      emissiveIntensity: tension * 0.2,
    });

    return { geometry: tubeGeometry, material: tubeMaterial };
  }, [tension, width, vibrationRef.current]);

  return (
    <mesh ref={meshRef} geometry={geometry} material={material} />
  );
}
