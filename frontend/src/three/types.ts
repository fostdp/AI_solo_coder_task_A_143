import * as THREE from 'three';

export interface CrossbowState {
  tension: number;
  bowArmAngle: number;
  magazinePosition: number;
  camRotation: number;
  pawlEngaged: boolean;
  loaded: boolean;
}

export interface ArrowState {
  id: number;
  position: THREE.Vector3;
  velocity: THREE.Vector3;
  active: boolean;
  trail: THREE.Vector3[];
  rotation: THREE.Euler;
}

export interface TrajectoryPoint {
  position: THREE.Vector3;
  time: number;
}

export interface PhysicsConfig {
  gravity: number;
  bowArmLength: number;
  maxTension: number;
  arrowMass: number;
  initialVelocity: number;
  launchAngle: number;
}

export interface CrossbowConfig {
  bodySize: {
    width: number;
    height: number;
    depth: number;
  };
  bowArm: {
    length: number;
    width: number;
    thickness: number;
  };
  magazine: {
    width: number;
    height: number;
    depth: number;
  };
  cam: {
    radius: number;
    height: number;
    teethCount: number;
  };
  pawl: {
    width: number;
    height: number;
    depth: number;
  };
  arrow: {
    length: number;
    radius: number;
  };
}

export interface BowStringProps {
  tension: number;
  width?: number;
  vibrationAmplitude?: number;
  vibrationDamping?: number;
}

export interface ArrowProps {
  position: THREE.Vector3;
  rotation?: THREE.Euler;
  velocity?: THREE.Vector3;
  showTrail?: boolean;
  trailLength?: number;
}

export interface TrajectoryLineProps {
  points: THREE.Vector3[];
  showImpactMarker?: boolean;
  maxPoints?: number;
}

export interface CrossbowModelProps {
  state: CrossbowState;
  config?: Partial<CrossbowConfig>;
  physicsConfig?: Partial<PhysicsConfig>;
  onShoot?: () => void;
}

export const DEFAULT_PHYSICS_CONFIG: PhysicsConfig = {
  gravity: 9.8,
  bowArmLength: 0.8,
  maxTension: 1.0,
  arrowMass: 0.02,
  initialVelocity: 50,
  launchAngle: Math.PI / 6,
};

export const DEFAULT_CROSSBOW_CONFIG: CrossbowConfig = {
  bodySize: {
    width: 0.1,
    height: 0.08,
    depth: 0.6,
  },
  bowArm: {
    length: 0.4,
    width: 0.05,
    thickness: 0.02,
  },
  magazine: {
    width: 0.08,
    height: 0.1,
    depth: 0.3,
  },
  cam: {
    radius: 0.06,
    height: 0.04,
    teethCount: 8,
  },
  pawl: {
    width: 0.03,
    height: 0.02,
    depth: 0.05,
  },
  arrow: {
    length: 0.4,
    radius: 0.015,
  },
};

export const WOOD_COLOR = '#8B4513';
export const METAL_COLOR = '#B8860B';
export const STRING_COLOR = '#F5F5DC';
export const STRING_TENSION_COLOR = '#FF6B35';
export const ARROW_COLOR = '#2F4F4F';
export const TRAIL_START_COLOR = '#FFD700';
export const TRAIL_END_COLOR = '#FF0000';
