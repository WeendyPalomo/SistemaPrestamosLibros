import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 20, // usuarios virtuales
  duration: '1m', // duración de la prueba
};

export default function () {
  let res = http.get('http://localhost:3000/libros'); 

  check(res, {
    'status is 200': (r) => r.status === 200,
  });

  sleep(1);
}
