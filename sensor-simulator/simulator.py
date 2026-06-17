#!/usr/bin/env python3
"""
连弩传感器模拟器
支持：HTTP上报 + MQTT上报，可调箭匣容量、弓弦疲劳等参数
"""
import argparse
import json
import logging
import os
import sys
import time
from datetime import datetime, timezone
from typing import Dict, Optional, Tuple

import numpy as np
import requests
import yaml

try:
    import paho.mqtt.client as mqtt
    HAS_MQTT = True
except ImportError:
    HAS_MQTT = False


class CrossbowSimulator:
    def __init__(self, config: Dict, args: argparse.Namespace):
        self.config = config
        self.args = args

        self.crossbow_id = args.crossbow_id or config.get('crossbow_id', 'crossbow-001')
        self.backend_url = args.backend_url or config.get('backend_url', 'http://localhost:8080/api/v1/sensor/data')
        self.report_interval = args.report_interval or config.get('report_interval', 1)
        self.noise_level = config.get('noise_level', 0.02)
        self.mode = args.mode or config.get('mode', 'normal')

        self.phys = config.get('physical_params', {})
        self.ranges = config.get('normal_ranges', {})
        self.scenario = config.get('scenario', {})

        self.shot_count = 0
        self.magazine_size = args.magazine_size or self.phys.get('magazine_size', 10)
        self.magazine_ammo = self.magazine_size

        self.fatigue_rate_per_shot = self.phys.get('fatigue_rate_per_shot', 0.001)
        self.tension_decay = self.phys.get('tension_decay_due_to_fatigue', 0.2)
        self.velocity_decay = self.phys.get('velocity_decay_due_to_fatigue', 0.25)

        self.string_fatigue = args.initial_fatigue
        if self.string_fatigue is None:
            self.string_fatigue = self.scenario.get('initial_string_fatigue', 0.0)

        self.break_at_fatigue = self.scenario.get('break_at_fatigue', 0.95)
        self.jam_after_shots = self.scenario.get('jam_after_shots', -1)
        self.cycle_period = self.scenario.get('cycle_period_seconds', 5.5)

        self.is_reloading = False
        self.reload_start_time = 0.0
        self.cycle_phase = 0.0
        self.last_shot_time = time.time()
        self.temperature_base = 25.0
        self.jammed = False
        self.string_broken = False

        self._setup_logging()
        self.mqtt_client: Optional[mqtt.Client] = None
        self._setup_mqtt()

    def _setup_logging(self):
        log_dir = os.path.join(os.path.dirname(__file__), 'logs')
        os.makedirs(log_dir, exist_ok=True)
        log_file = os.path.join(log_dir, f'sensor_{self.crossbow_id}.log')

        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
            handlers=[
                logging.FileHandler(log_file, encoding='utf-8'),
                logging.StreamHandler(sys.stdout)
            ]
        )
        self.logger = logging.getLogger(f'CrossbowSimulator.{self.crossbow_id}')

    def _setup_mqtt(self):
        mqtt_cfg = self.config.get('mqtt', {})
        if not mqtt_cfg.get('enabled', False) or self.args.disable_mqtt:
            self.logger.info("MQTT reporting disabled")
            return

        if not HAS_MQTT:
            self.logger.warning("paho-mqtt not installed, MQTT disabled. Install: pip install paho-mqtt")
            return

        try:
            broker = mqtt_cfg.get('broker', 'localhost')
            port = int(mqtt_cfg.get('port', 1883))
            qos = int(mqtt_cfg.get('qos', 1))
            client_id = mqtt_cfg.get('client_id', f'sim-{self.crossbow_id}').format(crossbow_id=self.crossbow_id)
            topic_template = mqtt_cfg.get('topic', 'crossbow/{crossbow_id}/sensor')
            self.mqtt_topic = topic_template.format(crossbow_id=self.crossbow_id)
            self.mqtt_qos = qos

            self.mqtt_client = mqtt.Client(client_id=client_id, clean_session=True)

            username = mqtt_cfg.get('username')
            password = mqtt_cfg.get('password')
            if username:
                self.mqtt_client.username_pw_set(username, password)

            if mqtt_cfg.get('use_tls', False):
                self.mqtt_client.tls_set()

            def on_connect(client, userdata, flags, rc):
                if rc == 0:
                    self.logger.info(f"MQTT connected to {broker}:{port}, topic={self.mqtt_topic}")
                else:
                    self.logger.error(f"MQTT connect failed, rc={rc}")

            def on_disconnect(client, userdata, rc):
                if rc != 0:
                    self.logger.warning(f"MQTT disconnected (rc={rc}), will auto-reconnect")

            self.mqtt_client.on_connect = on_connect
            self.mqtt_client.on_disconnect = on_disconnect

            self.mqtt_client.connect_async(broker, port, keepalive=60)
            self.mqtt_client.loop_start()
            self.logger.info(f"MQTT async connecting to {broker}:{port}")

        except Exception as e:
            self.logger.error(f"Failed to setup MQTT: {e}")
            self.mqtt_client = None

    def _add_noise(self, value: float, level: float = None) -> float:
        if level is None:
            level = self.noise_level
        noise = np.random.normal(0, level)
        return value * (1 + noise)

    def _clamp(self, value: float, min_val: float, max_val: float) -> float:
        return max(min_val, min(max_val, value))

    def _fatigue_factor_tension(self) -> float:
        return max(0.3, 1.0 - self.string_fatigue * self.tension_decay)

    def _fatigue_factor_velocity(self) -> float:
        return max(0.3, 1.0 - self.string_fatigue * self.velocity_decay)

    def _update_fatigue(self):
        mode = self.mode

        if mode == 'fatigue':
            delta = self.fatigue_rate_per_shot * 3
        elif mode == 'accelerated':
            delta = self.fatigue_rate_per_shot * 10
        else:
            delta = self.fatigue_rate_per_shot

        self.string_fatigue += delta
        self.string_fatigue = min(1.0, self.string_fatigue)

        if self.string_fatigue >= self.break_at_fatigue or mode == 'string_break':
            self.string_broken = True
            self.logger.warning("⚠️ 弓弦断裂！仿真进入断弦状态")

    def _calculate_tension(self, draw_position: float) -> float:
        if self.string_broken:
            return 0.0

        if self.jammed:
            return self._add_noise(800.0)

        tension_range = self.ranges.get('string_tension', {})
        idle = tension_range.get('idle', 450.0)
        max_tension = tension_range.get('max', 1100.0)

        stiffness = self.phys.get('string_stiffness', 3500.0)
        max_draw = 0.2
        stretch = draw_position * max_draw
        tension = idle + stiffness * stretch

        tension = self._clamp(tension, idle, max_tension)
        tension *= self._fatigue_factor_tension()

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

        velocity *= self._fatigue_factor_velocity()

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
        reload_per_shot = reload_time / self.magazine_size

        cycle_time = reload_time + fire_delay + reload_per_shot
        base_rate = 60.0 / cycle_time

        rate = base_rate * self._fatigue_factor_velocity()
        rate = self._add_noise(rate, 0.15)

        rate_range = self.ranges.get('fire_rate', {})
        return self._clamp(rate, rate_range.get('min', 8.5), rate_range.get('max', 11.5))

    def _update_cycle(self, delta_time: float) -> Tuple[float, float, float]:
        if self.string_broken:
            return 0.0, 0.0, 0.0

        if self.jammed:
            return 0.5, 0.5, 180.0

        if self.is_reloading:
            reload_dur = self.phys.get('reload_time', 5.5)
            if time.time() - self.reload_start_time >= reload_dur:
                self.is_reloading = False
                self.magazine_ammo = self.magazine_size
                self.logger.info(f"🔄 换弹完成，箭匣容量={self.magazine_size}")
            return 0.0, 0.0, 0.0

        cycle_speed = 1.0 / self.cycle_period
        self.cycle_phase += delta_time * cycle_speed

        if self.cycle_phase >= 1.0:
            self.cycle_phase = 0.0
            self.shot_count += 1
            self.magazine_ammo -= 1
            self._update_fatigue()
            self.last_shot_time = time.time()

            if self.jam_after_shots > 0 and self.shot_count >= self.jam_after_shots and not self.jammed:
                self.jammed = True
                self.logger.warning("⚠️ 机构卡死！已达到jam_after_shots阈值")

            self.logger.info(
                f"🏹 发射第 {self.shot_count:3d} 发 | "
                f"剩余: {self.magazine_ammo:2d}/{self.magazine_size} | "
                f"疲劳: {self.string_fatigue:.3f} | "
                f"{'⚠️ 卡弹' if self.jammed else ''}{'💔 断弦' if self.string_broken else ''}"
            )

            if self.magazine_ammo <= 0 and not self.string_broken:
                self.is_reloading = True
                self.reload_start_time = time.time()
                self.logger.info("📥 开始换弹...")

        if self.cycle_phase < 0.7:
            draw_position = self.cycle_phase / 0.7
            mag_position = draw_position
        elif self.cycle_phase < 0.9:
            draw_position = 1.0
            mag_position = 1.0
        else:
            draw_position = 1.0 - (self.cycle_phase - 0.9) / 0.1
            mag_position = max(0.0, 1.0 - (self.cycle_phase - 0.9) / 0.1)

        cam_angle = self.cycle_phase * 360.0

        return draw_position, mag_position, cam_angle

    def _update_temperature(self) -> float:
        temp_range = self.ranges.get('temperature', {})
        temp = self.temperature_base + 5.0 * np.sin(time.time() / 3600.0)
        temp += 2.0 * self.string_fatigue
        temp += np.random.normal(0, 0.5)
        return self._clamp(
            temp,
            temp_range.get('min', 20.0),
            temp_range.get('max', 45.0)
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

    def send_http(self, data: Dict) -> bool:
        max_retries = 3
        for attempt in range(max_retries):
            try:
                response = requests.post(
                    self.backend_url,
                    json=data,
                    timeout=5
                )
                response.raise_for_status()
                return True
            except requests.exceptions.RequestException as e:
                if attempt == 0:
                    self.logger.error(f"HTTP上报失败 (尝试 {attempt + 1}/{max_retries}): {e}")
                if attempt < max_retries - 1:
                    time.sleep(min(2 ** attempt, 3))
        return False

    def send_mqtt(self, data: Dict) -> bool:
        if not self.mqtt_client:
            return False
        try:
            payload = json.dumps(data, ensure_ascii=False)
            msg_info = self.mqtt_client.publish(
                self.mqtt_topic,
                payload=payload,
                qos=self.mqtt_qos,
                retain=False
            )
            return msg_info.rc == 0
        except Exception as e:
            self.logger.error(f"MQTT publish failed: {e}")
            return False

    def run(self):
        self.logger.info("=" * 60)
        self.logger.info("🏹 连弩传感器模拟器启动")
        self.logger.info("=" * 60)
        self.logger.info(f"连弩ID        : {self.crossbow_id}")
        self.logger.info(f"模拟模式      : {self.mode}")
        self.logger.info(f"上报间隔      : {self.report_interval}s")
        self.logger.info(f"箭匣容量      : {self.magazine_size} 发")
        self.logger.info(f"初始弓弦疲劳  : {self.string_fatigue:.3f}")
        self.logger.info(f"疲劳/发射    : {self.fatigue_rate_per_shot:.4f}")
        self.logger.info(f"HTTP后端      : {self.backend_url}")
        self.logger.info(f"MQTT          : {'启用' if self.mqtt_client else '禁用'}")
        self.logger.info("=" * 60)

        self._last_update_time = time.time()
        http_error_count = 0

        try:
            while True:
                if self.string_broken and self.mode not in ('string_break', 'accelerated'):
                    self.logger.warning("弓弦已断裂，自动停止（可使用 --mode string_break 继续模拟）")
                    break

                data = self.generate_sensor_data()

                http_ok = self.send_http(data)
                mqtt_ok = self.send_mqtt(data) if self.mqtt_client else None

                status_parts = []
                if http_ok:
                    status_parts.append("HTTP✓")
                else:
                    http_error_count += 1
                    status_parts.append(f"HTTP✗({http_error_count})")
                if mqtt_ok is not None:
                    status_parts.append("MQTT✓" if mqtt_ok else "MQTT✗")

                status_str = " ".join(status_parts)
                self.logger.info(
                    f"[{status_str}] "
                    f"张力={data['stringTension']:6.1f}N "
                    f"射速={data['fireRate']:5.1f}/m "
                    f"变形={data['bowArmDeformation']:5.2f}mm "
                    f"疲劳={data['stringFatigue']:.3f} "
                    f"温度={data['temperature']:4.1f}℃"
                )

                if http_error_count > 10:
                    self.logger.warning(f"HTTP连续失败{http_error_count}次，后端可能未就绪")

                time.sleep(self.report_interval)

        except KeyboardInterrupt:
            self.logger.info("模拟器被用户中断 (Ctrl+C)")
        except Exception as e:
            self.logger.exception(f"模拟器异常: {e}")
        finally:
            if self.mqtt_client:
                try:
                    self.mqtt_client.loop_stop()
                    self.mqtt_client.disconnect()
                except Exception:
                    pass
            self.logger.info(f"模拟器停止 | 总发射 {self.shot_count} 发 | 最终疲劳 {self.string_fatigue:.3f}")


def load_config(config_path: str) -> Dict:
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            return yaml.safe_load(f) or {}
    except Exception as e:
        print(f"加载配置文件失败: {e}", file=sys.stderr)
        return {}


def main():
    parser = argparse.ArgumentParser(
        description='🏹 连弩传感器模拟器 - 支持HTTP/MQTT双通道上报，可调箭匣容量和弓弦疲劳',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 正常模式
  python simulator.py

  # 加速疲劳模式（10倍疲劳增长）
  python simulator.py --mode accelerated

  # 小箭匣容量 + 初始高疲劳
  python simulator.py --magazine-size 5 --initial-fatigue 0.5

  # 故障注入：断弦模式
  python simulator.py --mode string_break

  # 故障注入：卡弹模式（50发后卡死）
  python simulator.py --jam-after 50
        """
    )
    parser.add_argument('--crossbow-id', type=str, help='连弩ID (默认从config.yaml读取)')
    parser.add_argument('--report-interval', type=float, help='上报间隔秒数 (默认1秒)')
    parser.add_argument('--backend-url', type=str, help='后端API地址')
    parser.add_argument('--mode', type=str,
                        choices=['normal', 'fatigue', 'accelerated', 'jammed', 'string_break'],
                        help='模拟模式')
    parser.add_argument('--config', type=str,
                        default=os.path.join(os.path.dirname(__file__), 'config.yaml'),
                        help='配置文件路径')

    sim_group = parser.add_argument_group('模拟器参数')
    sim_group.add_argument('--magazine-size', type=int,
                           help=f'箭匣容量 (默认10)')
    sim_group.add_argument('--initial-fatigue', type=float,
                           help='初始弓弦疲劳值 0.0~1.0 (默认0.0)')
    sim_group.add_argument('--fatigue-rate', type=float,
                           help='每发射疲劳增量 (覆盖配置文件)')
    sim_group.add_argument('--jam-after', type=int, default=-1,
                           help='发射N发后自动卡死, -1表示不触发 (默认-1)')
    sim_group.add_argument('--disable-mqtt', action='store_true',
                           help='禁用MQTT上报（即使配置中启用）')

    args = parser.parse_args()
    config = load_config(args.config)

    if args.fatigue_rate is not None:
        config.setdefault('physical_params', {})['fatigue_rate_per_shot'] = args.fatigue_rate
    if args.jam_after > 0:
        config.setdefault('scenario', {})['jam_after_shots'] = args.jam_after

    simulator = CrossbowSimulator(config, args)
    simulator.run()


if __name__ == '__main__':
    main()
