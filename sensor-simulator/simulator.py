#!/usr/bin/env python3
import argparse
import logging
import time
import yaml
import requests
import numpy as np
from datetime import datetime, timezone
from typing import Dict, Tuple
import sys
import os


class CrossbowSimulator:
    def __init__(self, config: Dict, args: argparse.Namespace):
        self.config = config
        self.args = args
        self.crossbow_id = args.crossbow_id or config.get('crossbow_id', 'crossbow-001')
        self.backend_url = args.backend_url or config.get('backend_url', 'http://localhost:8080/api/v1/sensor/data')
        self.report_interval = args.report_interval or config.get('report_interval', 60)
        self.noise_level = config.get('noise_level', 0.02)
        self.mode = args.mode

        self.phys = config.get('physical_params', {})
        self.ranges = config.get('normal_ranges', {})

        self.shot_count = 0
        self.magazine_ammo = self.phys.get('magazine_size', 10)
        self.string_fatigue = 0.0
        self.is_reloading = False
        self.reload_start_time = 0
        self.cycle_phase = 0.0
        self.last_shot_time = time.time()
        self.temperature_base = 25.0
        self.jammed = False
        self.string_broken = False
        self.fatigue_factor = 1.0

        self._setup_logging()

    def _setup_logging(self):
        log_dir = os.path.join(os.path.dirname(__file__), 'logs')
        os.makedirs(log_dir, exist_ok=True)
        log_file = os.path.join(log_dir, f'sensor_{self.crossbow_id}.log')

        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
            handlers=[
                logging.FileHandler(log_file),
                logging.StreamHandler(sys.stdout)
            ]
        )
        self.logger = logging.getLogger(f'CrossbowSimulator.{self.crossbow_id}')

    def _add_noise(self, value: float, level: float = None) -> float:
        if level is None:
            level = self.noise_level
        noise = np.random.normal(0, level)
        return value * (1 + noise)

    def _clamp(self, value: float, min_val: float, max_val: float) -> float:
        return max(min_val, min(max_val, value))

    def _update_fatigue(self):
        if self.mode == 'fatigue':
            self.string_fatigue += 0.003
            self.fatigue_factor = max(0.5, 1.0 - self.string_fatigue * 0.5)
        else:
            self.string_fatigue += 0.001
            self.fatigue_factor = max(0.8, 1.0 - self.string_fatigue * 0.2)

        if self.string_fatigue >= 0.95 or self.mode == 'string_break':
            self.string_broken = True
            self.logger.warning("弓弦断裂！")

    def _calculate_tension(self, draw_position: float) -> float:
        if self.string_broken:
            return 0.0

        if self.jammed:
            return self._add_noise(800.0)

        tension_range = self.ranges.get('string_tension', {})
        idle = tension_range.get('idle', 450.0)
        max_tension = tension_range.get('max', 1100.0)

        base_tension = idle + (max_tension - idle) * draw_position
        stiffness = self.phys.get('string_stiffness', 3500.0)
        stretch = draw_position * 0.2

        tension = idle + stiffness * stretch * (1 + np.random.normal(0, self.noise_level))
        tension = self._clamp(tension, idle, max_tension)

        if self.mode == 'fatigue':
            tension *= self.fatigue_factor

        return self._add_noise(tension)

    def _calculate_deformation(self, tension: float) -> float:
        if self.string_broken or tension <= 0:
            return 0.0

        L = self.phys.get('bow_arm_length', 0.45)
        E = self.phys.get('elastic_modulus', 70.0e9)
        I = self.phys.get('moment_of_inertia', 1.5e-8)

        force = tension * 0.5
        deformation_m = (force * L ** 3) / (3 * E * I)
        deformation_mm = deformation_m * 1000.0

        def_range = self.ranges.get('bow_arm_deformation', {})
        return self._clamp(
            self._add_noise(deformation_mm),
            def_range.get('min', 5.0),
            def_range.get('max', 18.0)
        )

    def _calculate_arrow_velocity(self, tension: float, draw_position: float) -> float:
        if self.string_broken or self.jammed or draw_position < 0.9:
            return 0.0

        stiffness = self.phys.get('string_stiffness', 3500.0)
        max_draw = 0.2
        efficiency = self.phys.get('energy_efficiency', 0.65)
        arrow_mass = self.phys.get('arrow_mass', 0.025)

        draw = draw_position * max_draw
        potential_energy = 0.5 * stiffness * draw ** 2
        kinetic_energy = efficiency * potential_energy
        velocity = np.sqrt(2 * kinetic_energy / arrow_mass)

        vel_range = self.ranges.get('arrow_velocity', {})
        return self._clamp(
            self._add_noise(velocity, 0.03),
            vel_range.get('min', 55.0),
            vel_range.get('max', 85.0)
        )

    def _calculate_fire_rate(self) -> float:
        if self.string_broken or self.jammed:
            return 0.0

        reload_time = self.phys.get('reload_time', 5.5)
        fire_delay = self.phys.get('fire_delay', 0.3)
        mag_size = self.phys.get('magazine_size', 10)
        reload_per_shot = reload_time / mag_size

        cycle_time = reload_time + fire_delay + reload_per_shot
        base_rate = 60.0 / cycle_time

        rate = base_rate * self.fatigue_factor
        rate = self._add_noise(rate, 0.15)

        rate_range = self.ranges.get('fire_rate', {})
        return self._clamp(rate, rate_range.get('min', 8.5), rate_range.get('max', 11.5))

    def _update_cycle(self, delta_time: float) -> Tuple[float, float, float]:
        if self.string_broken:
            return 0.0, 0.0, 0.0

        if self.jammed:
            return 0.5, 0.5, 180.0

        if self.is_reloading:
            if time.time() - self.reload_start_time >= 3.0:
                self.is_reloading = False
                self.magazine_ammo = self.phys.get('magazine_size', 10)
                self.logger.info("换弹完成")
            return 0.0, 0.0, 0.0

        cycle_speed = 1.0 / 6.0
        self.cycle_phase += delta_time * cycle_speed

        if self.cycle_phase >= 1.0:
            self.cycle_phase = 0.0
            self.shot_count += 1
            self.magazine_ammo -= 1
            self._update_fatigue()
            self.last_shot_time = time.time()
            self.logger.info(f"发射第 {self.shot_count} 发，剩余 {self.magazine_ammo} 发")

            if self.magazine_ammo <= 0:
                self.is_reloading = True
                self.reload_start_time = time.time()
                self.logger.info("开始换弹")

        if self.cycle_phase < 0.7:
            draw_position = self.cycle_phase / 0.7
            mag_position = draw_position
        elif self.cycle_phase < 0.9:
            draw_position = 1.0
            mag_position = 1.0
        else:
            draw_position = 1.0 - (self.cycle_phase - 0.9) / 0.1
            mag_position = 1.0 - (self.cycle_phase - 0.9) / 0.1

        cam_angle = self.cycle_phase * 360.0

        return draw_position, mag_position, cam_angle

    def _update_temperature(self) -> float:
        temp_range = self.ranges.get('temperature', {})
        temp = self.temperature_base + 5.0 * np.sin(time.time() / 3600.0)
        temp += np.random.normal(0, 0.5)
        return self._clamp(
            temp,
            temp_range.get('min', 20.0),
            temp_range.get('max', 30.0)
        )

    def generate_sensor_data(self) -> Dict:
        current_time = time.time()
        delta_time = current_time - getattr(self, '_last_update_time', current_time)
        self._last_update_time = current_time

        if self.mode == 'jammed':
            self.jammed = True

        draw_pos, mag_pos, cam_angle = self._update_cycle(delta_time)

        tension = self._calculate_tension(draw_pos)
        deformation = self._calculate_deformation(tension)
        velocity = self._calculate_arrow_velocity(tension, draw_pos)
        fire_rate = self._calculate_fire_rate()
        temperature = self._update_temperature()

        cam_range = self.ranges.get('cam_angle', {})
        cam_angle = self._clamp(cam_angle, cam_range.get('min', 0.0), cam_range.get('max', 360.0))

        fatigue_range = self.ranges.get('string_fatigue', {})
        fatigue = self._clamp(
            self.string_fatigue,
            fatigue_range.get('min', 0.0),
            fatigue_range.get('max', 1.0)
        )

        return {
            "crossbowId": self.crossbow_id,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "stringTension": round(tension, 2),
            "bowArmDeformation": round(deformation, 2),
            "magazinePosition": round(mag_pos, 3),
            "fireRate": round(fire_rate, 2),
            "arrowVelocity": round(velocity, 2),
            "camAngle": round(cam_angle, 1),
            "stringFatigue": round(fatigue, 4),
            "temperature": round(temperature, 1)
        }

    def send_data(self, data: Dict) -> bool:
        max_retries = 3
        for attempt in range(max_retries):
            try:
                response = requests.post(
                    self.backend_url,
                    json=data,
                    timeout=5
                )
                response.raise_for_status()
                self.logger.info(f"数据上报成功: 张力={data['stringTension']}N, 射速={data['fireRate']}发/分")
                return True
            except requests.exceptions.RequestException as e:
                self.logger.error(f"上报失败 (尝试 {attempt + 1}/{max_retries}): {e}")
                if attempt < max_retries - 1:
                    time.sleep(2 ** attempt)
        self.logger.error("达到最大重试次数，放弃本次上报")
        return False

    def run(self):
        self.logger.info(f"连弩传感器模拟器启动")
        self.logger.info(f"ID: {self.crossbow_id}")
        self.logger.info(f"模式: {self.mode}")
        self.logger.info(f"上报间隔: {self.report_interval}秒")
        self.logger.info(f"后端地址: {self.backend_url}")
        self._last_update_time = time.time()

        try:
            while True:
                if self.string_broken and self.mode != 'string_break':
                    self.logger.warning("弓弦已断裂，模拟器退出")
                    break

                data = self.generate_sensor_data()
                self.send_data(data)
                time.sleep(self.report_interval)

        except KeyboardInterrupt:
            self.logger.info("模拟器被用户中断")
        except Exception as e:
            self.logger.error(f"模拟器异常: {e}")
        finally:
            self.logger.info("模拟器停止")


def load_config(config_path: str) -> Dict:
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            return yaml.safe_load(f)
    except Exception as e:
        print(f"加载配置文件失败: {e}")
        return {}


def main():
    parser = argparse.ArgumentParser(description='连弩传感器模拟器')
    parser.add_argument('--crossbow-id', type=str, help='连弩ID')
    parser.add_argument('--report-interval', type=int, help='上报间隔(秒)')
    parser.add_argument('--backend-url', type=str, help='后端API地址')
    parser.add_argument('--mode', type=str,
                        choices=['normal', 'fatigue', 'jammed', 'string_break'],
                        default='normal',
                        help='模拟模式: normal(正常), fatigue(疲劳), jammed(卡弹), string_break(断弦)')
    parser.add_argument('--config', type=str,
                        default=os.path.join(os.path.dirname(__file__), 'config.yaml'),
                        help='配置文件路径')

    args = parser.parse_args()
    config = load_config(args.config)

    simulator = CrossbowSimulator(config, args)
    simulator.run()


if __name__ == '__main__':
    main()
